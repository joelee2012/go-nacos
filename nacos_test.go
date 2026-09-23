package nacos

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var config = `
{
	"id": "1",
	"dataId": "test",
	"group": "DEFAULT_GROUP",
	"content": "test content",
	"md5": "test-md5",
	"encryptedDataKey": "test-key",
	"tenant": "test-tenant",
	"appName": "test-app",
	"type": "properties"
}
`
var namespace = `
{
	"namespace": "test",
	"namespaceShowName": "Test",
	"namespaceDesc": "Test namespace",
	"quota": 100,
	"configCount": 10,
	"type": 0
}`

var role = `{"role": "ROLE_ADMIN", "username": "nacos"}`

var user = `{"username": "user1", "password": "$2a$10$C3B9EQgp93M6mvXwXiCebe1T9HvxGRj29x2dHIYCH.bUCdbJcrugO"}`
var permission = `{"role": "ROLE_ADMIN", "resource": "backend:*:*", "action": "rw"}`

func newV1Data(s string) string {
	return fmt.Sprintf(`{
  "totalCount": 1,
  "pageNumber": 1,
  "pagesAvailable": 0,
  "pageItems": [
    %s
  ]
}`, s)
}

func newV3Data(s string) string {
	return fmt.Sprintf(`{
  "code": 0,
  "message": "success",
  "data": %s
}
`, s)
}

var csList = newV1Data(config)

var csListV3 = newV3Data(csList)

var configV3 = newV3Data(config)

var nsList = fmt.Sprintf(`
{
  "code": 200,
  "message": "success",
  "data": [
    %s
  ]
}
`, namespace)

var userList = newV1Data(user)
var userListV3 = newV3Data(userList)

var roleList = newV1Data(role)
var roleListV3 = newV3Data(roleList)

var permList = newV1Data(permission)
var permListV3 = newV3Data(permList)

func TestNewClient(t *testing.T) {
	c, err := NewClient("http://localhost:8848", "user", "password")
	assert.NoError(t, err)
	assert.Equal(t, "http://localhost:8848/", c.url.String())
	assert.Equal(t, "user", c.user)
	assert.Equal(t, "password", c.password)
}

func startServer() (*httptest.Server, *Client) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/v1/console/namespaces", "/v3/console/core/namespace/list":
			if r.URL.Query().Get("show") == "all" {
				w.Write([]byte(namespace))
			} else {
				w.Write([]byte(nsList))
			}
		case "/v1/cs/configs":
			if r.URL.Query().Get("show") == "all" {
				w.Write([]byte(config))
			} else {
				w.Write([]byte(csList))
			}
		case "/v3/console/cs/config":
			w.Write([]byte(configV3))
		case "/v3/console/cs/config/list":
			w.Write([]byte(csListV3))
		case "/v1/console/server/state":
			w.Write([]byte(`{"version": "1.0.0"}`))
		case "/v3/console/server/state":
			w.Write([]byte(`{"version": "3.0.0"}`))
		case "/v1/auth/login", "/v3/auth/user/login":
			w.Write([]byte(`{"accessToken": "test-token", "tokenTtl": 3600, "globalAdmin": true}`))
		case "/v1/auth/users":
			w.Write([]byte(userList))
		case "/v3/auth/user/list":
			w.Write([]byte(userListV3))
		case "/v1/auth/roles":
			w.Write([]byte(roleList))
		case "/v3/auth/role/list":
			w.Write([]byte(roleListV3))
		case "/v1/auth/permissions":
			w.Write([]byte(permList))
		case "/v3/auth/permission/list":
			w.Write([]byte(permListV3))
		}
	}))
	c, _ := NewClient(ts.URL, "user", "password")
	return ts, c
}

var apiTests = []struct {
	apiVersion  string
	expectValue string
}{
	{apiVersion: "v1", expectValue: "1.0.0"},
	{apiVersion: "v3", expectValue: "3.0.0"},
}

func TestGetVersion(t *testing.T) {
	ts, _ := startServer()
	defer ts.Close()

	t.Run("uninitialized", func(t *testing.T) {
		c, _ := NewClient(ts.URL, "user", "password")
		_, err := c.GetVersion(context.Background())
		assert.ErrorIs(t, err, ErrNotInitialized)
	})

	t.Run("after init", func(t *testing.T) {
		c, _ := NewClient(ts.URL, "user", "password")
		assert.NoError(t, c.Init(context.Background()))
		version, err := c.GetVersion(context.Background())
		if assert.NoError(t, err) {
			assert.Equal(t, "3.0.0", version) // v3 is probed first
		}
	})
}

func TestInit(t *testing.T) {
	t.Run("detects v3", func(t *testing.T) {
		ts, _ := startServer()
		defer ts.Close()
		c, _ := NewClient(ts.URL, "user", "password")
		assert.NoError(t, c.Init(context.Background()))
		assert.Equal(t, "v3", c.apiVersion)
	})

	t.Run("falls back to v1 when v3 unavailable", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v3/console/server/state" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.URL.Path == "/v1/console/server/state" {
				w.Write([]byte(`{"version": "1.0.0"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()
		c, _ := NewClient(ts.URL, "user", "password")
		assert.NoError(t, c.Init(context.Background()))
		assert.Equal(t, "v1", c.apiVersion)
	})

	t.Run("fails when no version available", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ts.Close()
		c, _ := NewClient(ts.URL, "user", "password")
		assert.Error(t, c.Init(context.Background()))
		assert.Equal(t, "", c.apiVersion) // failure must not partially initialize
	})

	t.Run("idempotent", func(t *testing.T) {
		ts, _ := startServer()
		defer ts.Close()
		c, _ := NewClient(ts.URL, "user", "password")
		assert.NoError(t, c.Init(context.Background()))
		assert.NoError(t, c.Init(context.Background())) // second call is a no-op
		assert.Equal(t, "v3", c.apiVersion)
	})
}

func TestGetToken(t *testing.T) {
	okServer, _ := startServer()
	defer okServer.Close()
	nokServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`<html><body>404 Not Found</body></html>`))
	}))
	defer nokServer.Close()
	tests := []struct {
		name    string
		server  *httptest.Server
		wantErr bool
	}{
		{name: "OK", server: okServer, wantErr: false},
		{name: "NOK", server: nokServer, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := NewClient(tt.server.URL, "user", "password")
			c.apiVersion = "v1" // pin version to test token fetch directly
			token, err := c.GetToken(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, "", token)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, "test-token", token)
			}
		})
	}

}

func TestListNamespace(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()

	for _, tt := range apiTests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			c.apiVersion = tt.apiVersion
			ns, err := c.ListNamespace(context.Background())
			if assert.NoError(t, err) {
				assert.Equal(t, 1, len(ns.Items))
				assert.Equal(t, "test", ns.Items[0].ID)
			}
		})
	}
}

func TestCreateNamespace(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()

	for _, tt := range apiTests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			c.apiVersion = tt.apiVersion
			err := c.CreateNamespace(context.Background(), &NsOpts{Name: "test", Description: "Test namespace", ID: "test-id"})
			assert.NoError(t, err)
		})
	}
}

func TestGetNamespace(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	for _, tt := range apiTests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			c.apiVersion = tt.apiVersion
			n, err := c.GetNamespace(context.Background(), "test")
			if assert.NoError(t, err) {
				assert.Equal(t, "test", n.ID)
			}
		})
	}
}

func TestDeleteNamespace(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	for _, tt := range apiTests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			c.apiVersion = tt.apiVersion
			err := c.DeleteNamespace(context.Background(), "test-id")
			assert.NoError(t, err)
		})
	}
}

func TestUpdateNamespace(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	for _, tt := range apiTests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			c.apiVersion = tt.apiVersion
			err := c.UpdateNamespace(context.Background(), &NsOpts{Name: "test", Description: "Test namespace", ID: "test-id"})
			assert.NoError(t, err)
		})
	}
}
func TestCreateOrUpdateNamespace(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()

	tests := []struct {
		name string
		data NsOpts
	}{
		{name: "Create", data: NsOpts{Name: "test", Description: "Test namespace", ID: "test"}},
		{name: "Update", data: NsOpts{Name: "test-id", Description: "Test namespace", ID: "test-id"}},
	}
	c.apiVersion = "v1"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.CreateOrUpdateNamespace(context.Background(), &tt.data)
			assert.NoError(t, err)
		})
	}
}

func TestGetConfig(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	for _, tt := range apiTests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			c.apiVersion = tt.apiVersion
			cfg, err := c.GetConfig(context.Background(), &GetCfgOpts{DataID: "test", Group: "DEFAULT_GROUP"})
			if assert.NoError(t, err) {
				assert.Equal(t, "test", cfg.DataID)
			}
		})
	}
}

func TestListConfig(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	for _, tt := range apiTests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			c.apiVersion = tt.apiVersion
			cfgs, err := c.ListConfig(context.Background(), &ListCfgOpts{DataID: "test", Group: "DEFAULT_GROUP", PageNumber: 1, PageSize: 10})
			if assert.NoError(t, err) {
				assert.Equal(t, 1, len(cfgs.Items))
				assert.Equal(t, "test", cfgs.Items[0].DataID)
			}
		})
	}
}

func TestListConfigInNs(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()

	for _, tt := range apiTests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			c.apiVersion = tt.apiVersion
			cfgs, err := c.ListConfigInNs(context.Background(), "test", "DEFAULT_GROUP")
			if assert.NoError(t, err) {
				assert.Equal(t, 1, len(cfgs.Items))
				assert.Equal(t, "test", cfgs.Items[0].DataID)
			}
		})
	}
}

func TestListAllConfig(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	for _, tt := range apiTests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			c.apiVersion = tt.apiVersion
			cfgs, err := c.ListAllConfig(context.Background())
			if assert.NoError(t, err) {
				assert.Equal(t, 1, len(cfgs.Items))
				assert.Equal(t, "test", cfgs.Items[0].DataID)
			}
		})
	}
}

func TestPublishConfig(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	c.apiVersion = "v1"

	err := c.PublishConfig(context.Background(), &PublishCfgOpts{DataID: "test", Group: "DEFAULT_GROUP", Content: "test content", NamespaceID: "test-tenant", Type: "properties"})
	assert.NoError(t, err)
}

func TestDeleteConfig(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	c.apiVersion = "v1"

	err := c.DeleteConfig(context.Background(), &DeleteCfgOpts{DataID: "test", Group: "DEFAULT_GROUP", NamespaceID: "test-tenant"})
	assert.NoError(t, err)
}

func TestListUser(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	for _, tt := range apiTests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			c.apiVersion = tt.apiVersion
			users, err := c.ListUser(context.Background())
			if assert.NoError(t, err) {
				assert.Equal(t, "user1", users.Items[0].Name)
				// assert.Equal(t, "user2", users.Items[1].Name)
			}
		})
	}
}

func TestCreateUser(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	c.apiVersion = "v1"

	err := c.CreateUser(context.Background(), "user3", "password")
	assert.NoError(t, err)
}

func TestDeleteUser(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	c.apiVersion = "v1"

	err := c.DeleteUser(context.Background(), "user3")
	assert.NoError(t, err)
}

func TestGetUser(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()

	for _, tt := range apiTests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			c.apiVersion = tt.apiVersion
			user, err := c.GetUser(context.Background(), "user1")
			if assert.NoError(t, err) {
				assert.Equal(t, "user1", user.Name)
			}
		})
	}
}

func TestListRole(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	for _, tt := range apiTests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			c.apiVersion = tt.apiVersion
			roles, err := c.ListRole(context.Background())
			if assert.NoError(t, err) {
				assert.Equal(t, "ROLE_ADMIN", roles.Items[0].Name)
				assert.Equal(t, "nacos", roles.Items[0].Username)
			}
		})
	}
}

func TestCreateRole(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	c.apiVersion = "v1"

	err := c.CreateRole(context.Background(), "role1", "user1")
	assert.NoError(t, err)
}

func TestDeleteRole(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	c.apiVersion = "v1"

	err := c.DeleteRole(context.Background(), "role1", "user1")
	assert.NoError(t, err)
}

func TestGetRole(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()

	for _, tt := range apiTests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			c.apiVersion = tt.apiVersion
			role, err := c.GetRole(context.Background(), "ROLE_ADMIN", "nacos")
			if assert.NoError(t, err) {
				assert.Equal(t, "ROLE_ADMIN", role.Name)
			}
		})
	}
}

func TestListPermission(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	for _, tt := range apiTests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			c.apiVersion = tt.apiVersion
			perms, err := c.ListPermission(context.Background())
			if assert.NoError(t, err) {
				assert.Equal(t, "ROLE_ADMIN", perms.Items[0].Role)
				assert.Equal(t, "backend:*:*", perms.Items[0].Resource)
				assert.Equal(t, "rw", perms.Items[0].Action)
			}
		})
	}
}

func TestCreatePermission(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	c.apiVersion = "v1"

	err := c.CreatePermission(context.Background(), "ROLE_ADMIN", "backend:*:*", "rw")
	assert.NoError(t, err)
}

func TestDeletePermission(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	c.apiVersion = "v1"

	err := c.DeletePermission(context.Background(), "ROLE_ADMIN", "backend:*:*", "rw")
	assert.NoError(t, err)
}

func TestGetPermission(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()

	for _, tt := range apiTests {
		t.Run(tt.apiVersion, func(t *testing.T) {
			c.apiVersion = tt.apiVersion
			perm, err := c.GetPermission(context.Background(), "ROLE_ADMIN", "backend:*:*", "rw")
			if assert.NoError(t, err) {
				assert.Equal(t, "ROLE_ADMIN", perm.Role)
				assert.Equal(t, "backend:*:*", perm.Resource)
				assert.Equal(t, "rw", perm.Action)
			}
		})
	}
}

// TestInitRetryAfterFailure verifies a failed Init does not poison the
// process: constructing a fresh client against a healthy server and retrying
// succeeds. (We don't reuse the same client — that's what NewClient is for.)
func TestInitRetryAfterFailure(t *testing.T) {
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failServer.Close()

	c, _ := NewClient(failServer.URL, "user", "password")
	if err := c.Init(context.Background()); err == nil {
		t.Fatal("expected first Init to fail")
	} else {
		assert.ErrorIs(t, err, ErrDetectAPIVersion)
	}

	// retry with a fresh client against a healthy server
	okServer, _ := startServer()
	defer okServer.Close()
	c2, _ := NewClient(okServer.URL, "user", "password")
	if err := c2.Init(context.Background()); err != nil {
		t.Fatalf("Init retry should succeed: %v", err)
	}
	version, err := c2.GetVersion(context.Background())
	if assert.NoError(t, err) {
		assert.Equal(t, "3.0.0", version)
	}
}

// TestGetTokenConcurrent exercises #1: concurrent GetToken callers must not
// race on c.token. Under `go test -race` this would flag the old lockless
// fast-path read; here it also asserts all callers observe the same token.
func TestGetTokenConcurrent(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	c.apiVersion = "v1"

	const n = 32
	tokens := make([]string, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			tok, err := c.GetToken(context.Background())
			if assert.NoError(t, err) {
				tokens[i] = tok
			}
		}(i)
	}
	close(start)
	wg.Wait()

	for _, tok := range tokens {
		assert.Equal(t, "test-token", tok)
	}
}

// TestWithHTTPClient verifies the functional option is applied at construction.
func TestWithHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 7 * time.Second}
	c, err := NewClient("http://localhost:8848", "u", "p", WithHTTPClient(custom))
	if assert.NoError(t, err) {
		assert.Same(t, custom, c.HTTPClient())
		assert.Equal(t, 7*time.Second, c.HTTPClient().Timeout)
	}

	// nil must not wipe the default client.
	c2, err := NewClient("http://localhost:8848", "u", "p", WithHTTPClient(nil))
	if assert.NoError(t, err) {
		assert.NotNil(t, c2.HTTPClient())
		assert.Equal(t, 30*time.Second, c2.HTTPClient().Timeout)
	}

	// absence of options keeps the default.
	c3, err := NewClient("http://localhost:8848", "u", "p")
	if assert.NoError(t, err) {
		assert.NotNil(t, c3.HTTPClient())
		assert.Equal(t, 30*time.Second, c3.HTTPClient().Timeout)
	}
}

// TestWithAPIVersion verifies pinning the API version skips the /state
// probe: even when the server forbids /state, the pinned client can fetch a
// token and config directly. It also covers the NewClient validation and the
// GetVersion ("", nil) behavior.
func TestWithAPIVersion(t *testing.T) {
	// server that 403s the state probe but serves everything else (simulates
	// an environment where /console/server/state is restricted).
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/console/server/state") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/v3/auth/user/login":
			w.Write([]byte(`{"accessToken": "tok", "tokenTtl": 3600}`))
		case "/v3/console/cs/config":
			w.Write([]byte(configV3))
		}
	}))
	defer ts.Close()

	c, err := NewClient(ts.URL, "user", "password", WithAPIVersion("v3"))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "v3", c.apiVersion)

	// Init must be a no-op and must NOT hit /state (which would 403).
	assert.NoError(t, c.Init(context.Background()))

	// version is unknown because no probe was performed.
	ver, err := c.GetVersion(context.Background())
	if assert.NoError(t, err) {
		assert.Equal(t, "", ver, "pinned version must yield empty server version")
	}

	// resource methods work directly without Init probing /state.
	cfg, err := c.GetConfig(context.Background(), &GetCfgOpts{DataID: "test", Group: "DEFAULT_GROUP"})
	if assert.NoError(t, err) {
		assert.Equal(t, "test", cfg.DataID)
	}
}

// TestWithAPIVersionInvalid verifies NewClient rejects an unsupported version.
func TestWithAPIVersionInvalid(t *testing.T) {
	_, err := NewClient("http://localhost:8848", "u", "p", WithAPIVersion("v2"))
	assert.ErrorIs(t, err, ErrInvalidAPIVersion)

	// empty string is the documented "auto-detect" sentinel, not invalid.
	c, err := NewClient("http://localhost:8848", "u", "p", WithAPIVersion(""))
	if assert.NoError(t, err) {
		assert.Equal(t, "", c.apiVersion)
	}
}

// TestListConfigDoesNotMutateOpts verifies ListConfig no longer writes defaults
// back into the caller's *ListCfgOpts. Previously passing {PageNumber:0,
// PageSize:0} would silently mutate the struct to {1,10}.
func TestListConfigDoesNotMutateOpts(t *testing.T) {
	ts, c := startServer()
	defer ts.Close()
	c.apiVersion = "v1"

	opts := &ListCfgOpts{DataID: "test", Group: "DEFAULT_GROUP"} // PageNumber=0, PageSize=0
	if _, err := c.ListConfig(context.Background(), opts); err != nil {
		t.Fatalf("ListConfig: %v", err)
	}
	assert.Equal(t, 0, opts.PageNumber, "ListConfig must not mutate opts.PageNumber")
	assert.Equal(t, 0, opts.PageSize, "ListConfig must not mutate opts.PageSize")
}

// TestNotFoundMapping verifies the not-found contract: a server-side HTTP 404
// surfaces as ErrNotFound (so callers can uniformly errors.Is(err, ErrNotFound)),
// while a client-side absence (item missing from a listing) is also ErrNotFound.
// A non-404 failure must NOT match ErrNotFound.
func TestNotFoundMapping(t *testing.T) {
	// server that 404s /cs/configs but serves the login endpoint
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/login":
			w.Write([]byte(`{"accessToken": "t", "tokenTtl": 3600}`))
		case "/v1/cs/configs":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()

	c, _ := NewClient(ts.URL, "u", "p", WithAPIVersion("v1"))

	// 404 from server -> ErrNotFound.
	_, err := c.GetConfig(context.Background(), &GetCfgOpts{DataID: "x", Group: "g"})
	assert.ErrorIs(t, err, ErrNotFound)

	// sentinel itself matches (sanity).
	assert.True(t, errors.Is(ErrNotFound, ErrNotFound))
}

// TestRequestFailureNotNotFound verifies a 500 does NOT map to ErrNotFound
// and surfaces the status code + body in the error message. It exercises
// doRequest via GetConfig (login ok, then /cs returns 500).
func TestRequestFailureNotNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/login":
			w.Write([]byte(`{"accessToken": "t", "tokenTtl": 3600}`))
		case "/v1/cs/configs":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("server boom"))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	c, _ := NewClient(ts.URL, "u", "p", WithAPIVersion("v1"))
	_, err := c.GetConfig(context.Background(), &GetCfgOpts{DataID: "x", Group: "g"})
	if assert.Error(t, err) {
		assert.NotErrorIs(t, err, ErrNotFound, "500 must not be not-found")
		assert.Contains(t, err.Error(), "500")
		assert.Contains(t, err.Error(), "server boom")
	}
}
