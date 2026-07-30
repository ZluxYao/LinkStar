package stun_api

import (
	"encoding/json"
	"linkstar/modules/stun"
	"linkstar/modules/stun/model"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGetNatTypeView(t *testing.T) {
	previousUDP := stun.Runtime.Network.NatStatuUDP
	previousTCP := stun.Runtime.Network.NatStatuTCP
	t.Cleanup(func() {
		stun.Runtime.Network.NatStatuUDP = previousUDP
		stun.Runtime.Network.NatStatuTCP = previousTCP
	})

	stun.Runtime.Network.NatStatuUDP = &model.NatDetectResult{
		NatType:   "NAT1",
		Mapping:   "EIM",
		Filtering: "EIF",
	}
	stun.Runtime.Network.NatStatuTCP = &model.NatDetectResult{
		NatType:   "NAT3",
		Mapping:   "EIM",
		Filtering: "unknown",
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	(StunApi{}).GetNatTypeView(context)

	var response struct {
		Code int                 `json:"code"`
		Data NatTypeViewResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != 0 {
		t.Fatalf("code = %d, want 0", response.Code)
	}
	if response.Data.UDP == nil || response.Data.UDP.Mapping != "EIM" {
		t.Fatalf("unexpected UDP result: %#v", response.Data.UDP)
	}
	if response.Data.TCP == nil || response.Data.TCP.NatType != "NAT3" {
		t.Fatalf("unexpected TCP result: %#v", response.Data.TCP)
	}
}
