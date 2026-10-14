import type { ProxyEntry, ProxySite } from '../types'

/** 80 和 443 不用写出来，写了反而显得脏 */
export const portText = (https: boolean, port: number) => ((https ? port === 443 : port === 80) ? '' : `:${port}`)

/**
 * 站点对外的地址：自己占端口就用那个，否则挂在默认入口上。
 *
 * 挂默认入口且勾了 HTTPS 时，HTTP 入口上其实也有一份——
 * 但用户真正会用的是 HTTPS 那个，这里就显示它。
 */
export function siteAddr(entry: ProxyEntry, site: ProxySite): { https: boolean; port: number } | null {
  if (site.listenPort > 0) return { https: site.https, port: site.listenPort }
  if (site.https && entry.httpsPort > 0) return { https: true, port: entry.httpsPort }
  if (entry.httpPort > 0) return { https: false, port: entry.httpPort }
  return null
}

/** 可点的访问链接，用第一个域名。通配域名拼不出具体地址，返回空 */
export function siteURL(entry: ProxyEntry, site: ProxySite): string {
  const addr = siteAddr(entry, site)
  const host = site.hosts?.[0] ?? ''
  if (!addr || !host || host.startsWith('*')) return ''
  return `${addr.https ? 'https' : 'http'}://${host}${portText(addr.https, addr.port)}${site.pathPrefix}`
}
