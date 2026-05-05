package api

import (
	"encoding/json"
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
	"go.sia.tech/indexd/slabs"

	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal-plugin-sia/internal"
	pluginConfig "go.lumeweb.com/portal-plugin-sia/internal/config"
	"go.lumeweb.com/portal-plugin-sia/internal/testing/mocks"
	coreTesting "go.lumeweb.com/portal/core/testing"
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
		w.Write([]byte("<html><body>Auth Connect Page</body></html>"))
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(nil)
	})

	// POST /auth/connect/{requestID} - Approve Connection (basic auth)
	mux.HandleFunc("POST /auth/connect/{requestID}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
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

// TestOptions provides test configuration for API tests
var TestOptions = coreTesting.CombineOptions(
	coreTesting.WithMockServiceFactory(pluginCore.SIA_SERVICE, mocks.NewMockSiaService),
	coreTesting.WithMockServiceFactory(pluginCore.QUOTA_SERVICE, mocks.NewMockQuotaService),
	coreTesting.WithHTTPService(),
	coreTesting.WithPlugins(),
	coreTesting.WithAPIConfig(internal.ProtocolName, &pluginConfig.APIConfig{}),
)
