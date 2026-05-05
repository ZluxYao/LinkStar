package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/quic-go/quic-go/http3"
)

const defaultAddr = "127.0.0.1:8445"
const browserHost = "quic.local.test"
const certFile = "quic-localhost-cert.pem"
const keyFile = "quic-localhost-key.pem"

var page = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>QUIC Only Web</title>
  <style>
    :root {
      color-scheme: dark;
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: #101820;
      color: #f6f8f8;
    }
    * { box-sizing: border-box; }
    body {
      min-height: 100vh;
      margin: 0;
      display: grid;
      place-items: center;
      padding: 32px;
      background:
        radial-gradient(circle at 20% 20%, rgba(255, 183, 77, 0.2), transparent 28%),
        linear-gradient(135deg, #101820 0%, #163832 48%, #0f1419 100%);
    }
    main {
      width: min(880px, 100%);
      border: 1px solid rgba(255, 255, 255, 0.14);
      border-radius: 8px;
      padding: clamp(28px, 6vw, 56px);
      background: rgba(16, 24, 32, 0.72);
      box-shadow: 0 24px 80px rgba(0, 0, 0, 0.32);
    }
    .eyebrow {
      margin: 0 0 14px;
      color: #ffcf70;
      font-size: 14px;
      font-weight: 700;
      letter-spacing: 0;
      text-transform: uppercase;
    }
    h1 {
      margin: 0;
      font-size: clamp(38px, 7vw, 72px);
      line-height: 0.96;
      letter-spacing: 0;
    }
    p {
      max-width: 680px;
      margin: 22px 0 0;
      color: #d6dfdc;
      font-size: clamp(17px, 2.2vw, 22px);
      line-height: 1.65;
    }
    dl {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
      gap: 14px;
      margin: 34px 0 0;
    }
    div {
      border-left: 3px solid #ffcf70;
      padding: 4px 0 4px 14px;
    }
    dt {
      color: #90a8a1;
      font-size: 13px;
      text-transform: uppercase;
      letter-spacing: 0;
    }
    dd {
      margin: 6px 0 0;
      font-size: 18px;
      font-weight: 700;
      overflow-wrap: anywhere;
    }
    code {
      color: #ffe3a6;
      font-family: "Cascadia Code", "Fira Code", Consolas, monospace;
    }
  </style>
</head>
<body>
  <main>
    <p class="eyebrow">HTTP/3 over QUIC</p>
    <h1>{{.Title}}</h1>
    <p>
      {{.Message}}
    </p>
    <dl>
      <div>
        <dt>Protocol</dt>
        <dd>{{.Proto}}</dd>
      </div>
      <div>
        <dt>Transport</dt>
        <dd>{{.Transport}}</dd>
      </div>
      <div>
        <dt>Remote</dt>
        <dd>{{.RemoteAddr}}</dd>
      </div>
      {{if .AltSvc}}
      <div>
        <dt>Alt-Svc</dt>
        <dd>{{.AltSvc}}</dd>
      </div>
      {{end}}
      <div>
        <dt>Time</dt>
        <dd>{{.Now}}</dd>
      </div>
    </dl>
  </main>
</body>
</html>
`))

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	mode := "redirect"
	if len(args) > 0 && args[0] != "" {
		mode = args[0]
		args = args[1:]
	}

	switch mode {
	case "serve":
		return serve(args)
	case "redirect":
		return redirect(args)
	case "get":
		return get(args)
	case "chrome":
		return openChrome(args)
	case "help", "-h", "--help":
		usage(os.Stdout)
		return nil
	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown command %q", mode)
	}
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", defaultAddr, "UDP address for the HTTP/3 server")
	if err := fs.Parse(args); err != nil {
		return err
	}

	tlsConfig, spki, err := devTLSConfig()
	if err != nil {
		return err
	}

	server := http3.Server{
		Addr:      *addr,
		TLSConfig: tlsConfig,
		Handler:   quicOnlyHandler(),
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("HTTP/3 server listening on https://%s (UDP only)", *addr)
		log.Printf("test with: go run . get -url https://%s/", *addr)
		log.Printf("open Chrome with: go run . chrome -force -addr %s", *addr)
		log.Printf("certificate SPKI: %s", spki)
		errCh <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-stop:
		log.Printf("received %s, shutting down", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func redirect(args []string) error {
	fs := flag.NewFlagSet("redirect", flag.ExitOnError)
	tcpAddr := fs.String("tcp", defaultAddr, "TCP HTTPS address that advertises Alt-Svc")
	udpAddr := fs.String("udp", defaultAddr, "UDP HTTP/3 address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	tlsConfig, spki, err := devTLSConfig()
	if err != nil {
		return err
	}

	_, udpPort, err := net.SplitHostPort(*udpAddr)
	if err != nil {
		return fmt.Errorf("parse udp addr %q: %w", *udpAddr, err)
	}
	altSvc := fmt.Sprintf(`h3=":%s"; ma=86400`, udpPort)
	handler := altSvcHandler(altSvc)

	quicServer := http3.Server{
		Addr:      *udpAddr,
		TLSConfig: tlsConfig,
		Handler:   handler,
	}
	tcpServer := http.Server{
		Addr:      *tcpAddr,
		TLSConfig: tlsConfig,
		Handler:   handler,
	}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("HTTP/3 server listening on https://%s (UDP)", *udpAddr)
		errCh <- quicServer.ListenAndServe()
	}()
	go func() {
		log.Printf("Alt-Svc HTTPS server listening on https://%s (TCP)", *tcpAddr)
		log.Printf("Alt-Svc: %s", altSvc)
		log.Printf("Chrome SPKI: %s", spki)
		log.Printf("open browser with: go run . chrome -addr %s -url https://%s/", *udpAddr, browserOriginForAddr(*tcpAddr))
		errCh <- tcpServer.ListenAndServeTLS("", "")
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-stop:
		log.Printf("received %s, shutting down", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = quicServer.Shutdown(ctx)
		return tcpServer.Shutdown(ctx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func quicOnlyHandler() http.Handler {
	return webHandler("", "QUIC over UDP", "This page did not use TCP.", "The server listens on UDP only. The client connects directly with HTTP/3 over QUIC.")
}

func altSvcHandler(altSvc string) http.Handler {
	return webHandler(altSvc, "Alt-Svc advertised", "Alt-Svc bridge to QUIC.", "This HTTPS/TCP response advertises a QUIC endpoint with Alt-Svc. Reload or revisit this origin and a browser can switch future requests to HTTP/3.")
}

func webHandler(altSvc, transport, title, message string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if altSvc != "" {
			w.Header().Set("Alt-Svc", altSvc)
		}
		actualTransport := transport
		if r.ProtoMajor == 3 {
			actualTransport = "QUIC over UDP"
		} else if r.TLS != nil {
			actualTransport = "TLS over TCP"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Transport", actualTransport)
		if err := page.Execute(w, map[string]string{
			"Title":      title,
			"Message":    message,
			"Proto":      r.Proto,
			"Transport":  actualTransport,
			"RemoteAddr": r.RemoteAddr,
			"AltSvc":     altSvc,
			"Now":        time.Now().Format(time.RFC3339),
		}); err != nil {
			log.Printf("render page: %v", err)
		}
	})
	mux.HandleFunc("/api/transport", func(w http.ResponseWriter, r *http.Request) {
		actualTransport := transport
		if r.ProtoMajor == 3 {
			actualTransport = "QUIC over UDP"
		} else if r.TLS != nil {
			actualTransport = "TLS over TCP"
		}
		if altSvc != "" {
			w.Header().Set("Alt-Svc", altSvc)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Transport", actualTransport)
		fmt.Fprintf(w, `{"protocol":%q,"transport":%q,"remote":%q,"alt_svc":%q}`+"\n", r.Proto, actualTransport, r.RemoteAddr, altSvc)
	})
	return mux
}

func get(args []string) error {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	url := fs.String("url", "https://"+defaultAddr+"/", "HTTP/3 URL to fetch")
	if err := fs.Parse(args); err != nil {
		return err
	}

	transport := &http3.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{http3.NextProtoH3},
		},
	}
	defer transport.Close()

	client := http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	resp, err := client.Get(*url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	fmt.Printf("status: %s\n", resp.Status)
	fmt.Printf("protocol: %s\n", resp.Proto)
	fmt.Printf("x-transport: %s\n\n", resp.Header.Get("X-Transport"))
	fmt.Print(string(body))
	return nil
}

func openChrome(args []string) error {
	fs := flag.NewFlagSet("chrome", flag.ExitOnError)
	addr := fs.String("addr", defaultAddr, "UDP address of the running HTTP/3 server")
	page := fs.String("url", "", "URL to open; defaults to the forced QUIC origin")
	browser := fs.String("browser", "chrome", "chrome, edge, or a browser executable path")
	profile := fs.String("profile", filepath.Join(os.TempDir(), "quic-demo-chrome"), "temporary browser profile directory")
	force := fs.Bool("force", false, "force this origin to use QUIC immediately")
	if err := fs.Parse(args); err != nil {
		return err
	}

	_, spki, err := devTLSConfig()
	if err != nil {
		return err
	}

	host, port, err := net.SplitHostPort(*addr)
	if err != nil {
		return fmt.Errorf("parse addr %q: %w", *addr, err)
	}
	if host == "" {
		host = "127.0.0.1"
	}

	browserPath, err := findBrowser(*browser)
	if err != nil {
		return err
	}

	origin := net.JoinHostPort(browserHost, port)
	pageURL := url.URL{
		Scheme: "https",
		Host:   origin,
		Path:   "/",
	}
	openURL := pageURL.String()
	if *page != "" {
		openURL = *page
	}

	chromeArgs := []string{
		"--user-data-dir=" + *profile,
		"--no-proxy-server",
		"--enable-quic",
		"--host-resolver-rules=MAP " + browserHost + " " + host,
		"--ignore-certificate-errors",
		"--ignore-certificate-errors-spki-list=" + spki,
	}
	if *force {
		chromeArgs = append(chromeArgs, "--origin-to-force-quic-on="+origin)
	}
	chromeArgs = append(chromeArgs, openURL)

	cmd := exec.Command(browserPath, chromeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("opening %s\n", openURL)
	if *force {
		fmt.Printf("forcing QUIC origin: %s\n", origin)
	} else {
		fmt.Printf("Alt-Svc discovery origin: %s\n", origin)
	}
	fmt.Printf("using certificate SPKI: %s\n", spki)
	return cmd.Start()
}

func findBrowser(name string) (string, error) {
	if name != "chrome" && name != "edge" {
		if _, err := os.Stat(name); err == nil {
			return name, nil
		}
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("browser executable not found: %s", name)
	}

	candidates := []string{}
	if name == "chrome" {
		candidates = append(candidates,
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		)
	} else {
		candidates = append(candidates,
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "Application", "msedge.exe"),
		)
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("%s was not found", name)
}

func browserOriginForAddr(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return net.JoinHostPort(browserHost, "443")
	}
	return net.JoinHostPort(browserHost, port)
}

func devTLSConfig() (*tls.Config, string, error) {
	cert, spki, err := loadOrCreateDevCert()
	if err != nil {
		return nil, "", err
	}

	return http3.ConfigureTLSConfig(&tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{http3.NextProtoH3},
		MinVersion:   tls.VersionTLS13,
	}), spki, nil
}

func loadOrCreateDevCert() (tls.Certificate, string, error) {
	certPEM, certErr := os.ReadFile(certFile)
	keyPEM, keyErr := os.ReadFile(keyFile)
	if certErr != nil || keyErr != nil {
		var err error
		certPEM, keyPEM, err = generateDevCertPEM()
		if err != nil {
			return tls.Certificate{}, "", err
		}
		if err := os.WriteFile(certFile, certPEM, 0644); err != nil {
			return tls.Certificate{}, "", err
		}
		if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
			return tls.Certificate{}, "", err
		}
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return tls.Certificate{}, "", err
	}
	cert.Leaf = leaf

	hash := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	return cert, base64.StdEncoding.EncodeToString(hash[:]), nil
}

func generateDevCertPEM() ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	certTemplate := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "localhost QUIC demo",
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", browserHost},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
		},
	}

	der, err := x509.CreateCertificate(rand.Reader, &certTemplate, &certTemplate, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return certPEM, keyPEM, nil
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `QUIC-only web demo (%s/%s)

Commands:
  go run .                         Start TCP HTTPS plus UDP HTTP/3.
  go run . redirect                Start TCP HTTPS plus UDP HTTP/3.
  go run . serve [-addr %s]        Start a UDP-only HTTP/3 server.
  go run . get [-url https://%s/]  Fetch the page with an HTTP/3 client.
  go run . chrome [-addr %s]       Open Chrome for Alt-Svc discovery.

Notes:
  The default mode opens a TCP HTTPS bootstrap port and a UDP QUIC port, like
  nginx does for HTTP/3 Alt-Svc discovery. The "serve" mode is UDP-only and
  needs "go run . chrome -force" or an HTTP/3-capable curl:
  curl --http3-only -k https://%s/
`, runtime.GOOS, runtime.GOARCH, defaultAddr, defaultAddr, defaultAddr, defaultAddr)
}
