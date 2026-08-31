package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"lukechampine.com/frand"

	proto "go.sia.tech/core/rhp/v4"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/chain"
	"go.sia.tech/indexd/accounts"
	"go.sia.tech/indexd/api/app"
	"go.sia.tech/indexd/hosts"
	"go.sia.tech/indexd/sharing"
	"go.sia.tech/indexd/slabs"

	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal-plugin-sia/internal"
	pluginConfig "go.lumeweb.com/portal-plugin-sia/internal/config"
	"go.lumeweb.com/portal-plugin-sia/internal/testing/mocks"
	core "go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"github.com/stretchr/testify/mock"
)

const (
	nonexistentSlabID  = "nonexistent-slab-id"
	nonexistentObjKey  = "nonexistent-object-key"
)

var fakeIndexd *httptest.Server

func TestMain(m *testing.M) {
	// Setup fake indexd server using http.ServeMux
	mux := http.NewServeMux()

	// POST /slabs - Pin Slabs
	mux.HandleFunc("POST /slabs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		slabID := slabs.SlabID(types.Hash256{1, 2, 3})
		encoder.Encode([]slabs.SlabID{slabID})
	})

	// POST /objects - Pin Object
	mux.HandleFunc("POST /objects", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /account - Account Info
	mux.HandleFunc("GET /account", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		resp := app.AccountResponse{
			AccountKey:       proto.Account{},
			MaxPinnedData:    1000000000,
			RemainingStorage: 500000000,
			Ready:            true,
			PinnedData:       1000,
			PinnedSize:       2000,
			App: accounts.AppMeta{
				ID:          types.Hash256{1, 2, 3},
				Name:        "Test App",
				Description: "Test Description",
			},
			LastUsed: time.Now(),
		}
		encoder.Encode(resp)
	})

	// DELETE /slabs/{id} - Unpin Slab
	mux.HandleFunc("DELETE /slabs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == nonexistentSlabID {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// POST /slabs/prune - Prune Slabs
	mux.HandleFunc("POST /slabs/prune", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /hosts - List Hosts
	mux.HandleFunc("GET /hosts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		hostsList := []hosts.HostInfo{
			{
				PublicKey:     types.PublicKey{1, 2, 3},
				Addresses:     []chain.NetAddress{{Protocol: "tcp", Address: "127.0.0.1:9981"}},
				CountryCode:   "US",
				Latitude:      37.7749,
				Longitude:     -122.4194,
				GoodForUpload: true,
			},
		}
		encoder.Encode(hostsList)
	})

	// GET /slabs - List Slabs
	mux.HandleFunc("GET /slabs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		slabID := slabs.SlabID(types.Hash256{1, 2, 3})
		encoder.Encode([]slabs.SlabID{slabID})
	})

	// GET /slabs/{slabid} - Get Slab
	mux.HandleFunc("GET /slabs/{slabid}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		resp := slabs.PinnedSlab{
			ID:            slabs.SlabID(types.Hash256{1, 2, 3}),
			EncryptionKey: slabs.EncryptionKey(frand.Entropy256()),
			MinShards:     1,
			Sectors:       []slabs.PinnedSector{},
		}
		encoder.Encode(resp)
	})

	// GET /objects - List Objects
	mux.HandleFunc("GET /objects", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		encoder.Encode([]slabs.ObjectEvent{})
	})

	// GET /objects/{key} - Get Object
	mux.HandleFunc("GET /objects/{key}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		key1 := frand.Entropy256()
		key2 := frand.Entropy256()
		resp := slabs.SealedObject{
			EncryptedDataKey:     key1[:],
			Slabs:                []slabs.SlabSlice{},
			DataSignature:        types.Signature{1, 2, 3},
			EncryptedMetadataKey: key2[:],
			EncryptedMetadata:    []byte{},
			MetadataSignature:    types.Signature{4, 5, 6},
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		encoder.Encode(resp)
	})

	// GET /objects/{key}/shared - Get Shared Object
	mux.HandleFunc("GET /objects/{key}/shared", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		resp := slabs.SharedObject{
			Slabs: []slabs.SlabSlice{},
		}
		encoder.Encode(resp)
	})

	// POST /auth/connect - Register App
	mux.HandleFunc("POST /auth/connect", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		resp := app.RegisterAppResponse{
			ResponseURL: "http://localhost:8080/auth/response",
			StatusURL:   "http://localhost:8080/auth/status",
			RegisterURL: "http://localhost:8080/auth/register",
			Expiration:  time.Now().Add(5 * time.Minute),
		}
		encoder.Encode(resp)
	})

	// GET /auth/check - Check Auth
	mux.HandleFunc("GET /auth/check", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /auth/connect/{requestID} - Auth Connect UI
	mux.HandleFunc("GET /auth/connect/{requestID}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fakeIndexdAuthConnectHTML("Test App", "A test application", "https://example.com/logo.png", "https://example.com/callback")))
	})

	// GET /auth/connect/{requestID}/status - Auth Connect Status
	mux.HandleFunc("GET /auth/connect/{requestID}/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		resp := app.AuthConnectStatusResponse{
			Approved:   false,
			UserSecret: types.Hash256{},
		}
		encoder.Encode(resp)
	})

	// POST /auth/connect/{requestID}/register - Register Connection
	mux.HandleFunc("POST /auth/connect/{requestID}/register", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// POST /auth/connect/{requestID} - Approve/Reject Connection (basic auth)
	mux.HandleFunc("POST /auth/connect/{requestID}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Approve bool `json:"approve"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.Approve {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	})

	// DELETE /objects/{key} - Unpin Object
	mux.HandleFunc("DELETE /objects/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		if key == nonexistentObjKey {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /funding/events - Funding Events
	mux.HandleFunc("GET /funding/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		encoder.Encode([]accounts.FundingEvent{})
	})

	// POST /sharing - Create Sharing Key
	mux.HandleFunc("POST /sharing", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		encoder := json.NewEncoder(w)
		encoder.Encode(sharing.Key{PublicKey: types.PublicKey{1, 2, 3}})
	})

	// GET /sharing - List Sharing Keys
	mux.HandleFunc("GET /sharing", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		encoder.Encode([]sharing.Key{{PublicKey: types.PublicKey{1, 2, 3}}})
	})

	// GET /sharing/{key} - Get Sharing Key
	mux.HandleFunc("GET /sharing/{key}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		encoder.Encode(sharing.Key{PublicKey: types.PublicKey{1, 2, 3}})
	})

	// DELETE /sharing/{key} - Delete Sharing Key
	mux.HandleFunc("DELETE /sharing/{key}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// POST /sharing/{key}/objects - Attach Object
	mux.HandleFunc("POST /sharing/{key}/objects", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /sharing/{key}/objects - List Attached Objects
	mux.HandleFunc("GET /sharing/{key}/objects", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		encoder.Encode([]slabs.SealedObject{})
	})

	// DELETE /sharing/{key}/objects/{objectkey} - Detach Object
	mux.HandleFunc("DELETE /sharing/{key}/objects/{objectkey}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /shared - Sharing Key Stats
	mux.HandleFunc("GET /shared", func(w http.ResponseWriter, r *http.Request) {
		if !requireSharingKeyAuth(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		encoder.Encode(sharing.KeyStats{ObjectCount: 1})
	})

	// GET /shared/objects - List Shared Objects
	mux.HandleFunc("GET /shared/objects", func(w http.ResponseWriter, r *http.Request) {
		if !requireSharingKeyAuth(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		encoder.Encode([]slabs.SealedObject{})
	})

	// GET /shared/objects/{id} - Get Shared Object
	mux.HandleFunc("GET /shared/objects/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !requireSharingKeyAuth(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		encoder.Encode(slabs.SealedObject{})
	})

	// GET /shared/hosts - List Shared Hosts
	mux.HandleFunc("GET /shared/hosts", func(w http.ResponseWriter, r *http.Request) {
		if !requireSharingKeyAuth(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		encoder.Encode([]app.SharedHost{})
	})

	// Catch-all for unhandled routes - return error to catch missing handlers
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "route not handled by fake indexd: " + r.Method + " " + r.URL.Path})
	})

	fakeIndexd = httptest.NewServer(mux)
	defer fakeIndexd.Close()

	coreTesting.WithOptions(m,
		coreTesting.WithMockServiceFactory(pluginCore.SIA_SERVICE, mocks.NewMockSiaService),
		coreTesting.WithMockServiceFactory(pluginCore.QUOTA_SERVICE, mocks.NewMockQuotaService),
		coreTesting.WithAPI(internal.ProtocolName, NewAPI),
		coreTesting.WithAPIID(internal.ProtocolName),
		coreTesting.WithProtocolConfig(internal.ProtocolName, pluginConfig.ProtocolConfig{
			Key:    "test-admin-key",
			URL:    "http://localhost:8080",
			AppURL: fakeIndexd.URL,
		}),
	)
}

// requireSharingKeyAuth mirrors indexd's sharing-key authentication contract
// for the recipient /shared* routes: the request must carry the Sia signed-URL
// credential/signature/validUntil query params. When they are absent, it writes
// a 401 response and returns false, so callers should halt. The portal proxies
// these routes without its own auth middleware, so indexd enforces the sharing
// key signature; this guard keeps the mock faithful to that behavior.
func requireSharingKeyAuth(w http.ResponseWriter, r *http.Request) bool {
	q := r.URL.Query()
	if !q.Has(queryParamCredential) || !q.Has(queryParamSignature) || !q.Has(queryParamValidUntil) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return false
	}
	return true
}

func fakeIndexdAuthConnectHTML(name, description, logoURL, callbackURL string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><body><div class="overlay"></div><div id="modal" class="card" role="dialog" aria-modal="true" aria-labelledby="dlg-title"><div class="header"><div class="logo"><img src="%s" alt="app logo" style="width: 100%%; height: 100%%; object-fit: cover"/></div><div><h1 id="dlg-title" class="title">Connect to app?</h1><p class="subtitle">%s</p></div></div><div class="copy"><p class="subtitle">%s</p><p>Connecting to this application will allow it to upload and download data on your behalf.</p></div><div class="actions"><button id="rejectButton" class="btn btn-danger">Reject</button><button id="acceptButton" class="btn btn-primary">Accept</button></div></div><script>function respondToRequest(approve){}function renderResult(approved){const callbackUrl = "%s";if(callbackUrl)window.location.href=callbackUrl;}</script></body></html>`, logoURL, name, description, callbackURL)
}

// TestOptions provides test configuration for API tests
var TestOptions = coreTesting.CombineOptions(
	coreTesting.WrapCoreOption(core.ContextWithStartupFunc(func(ctx core.Context) error {
		mockHTTPSvc := coreTesting.GetMockHTTPService(ctx)
		mockHTTPSvc.EXPECT().Port().Return(uint16(443)).Maybe()
		mockHTTPSvc.EXPECT().APISubdomain(mock.AnythingOfType("string"), mock.AnythingOfType("bool")).Return("sia.example.com").Maybe()
		return nil
	})),
	coreTesting.WithMockServiceFactory(pluginCore.SIA_SERVICE, mocks.NewMockSiaService),
	coreTesting.WithMockServiceFactory(pluginCore.QUOTA_SERVICE, mocks.NewMockQuotaService),
	coreTesting.WithPlugins(),
	coreTesting.WithAPIConfig(internal.ProtocolName, &pluginConfig.APIConfig{}),
)
