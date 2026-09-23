package nacos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	url        *url.URL
	user       string
	password   string
	httpClient *http.Client
	token      *Token
	state      *State
	apiVersion string
	mu         sync.RWMutex // protects token refresh
	initMu     sync.Mutex   // protects Init; apiVersion/state are read-only after Init
}

// Option configures a Client at construction time.
type Option func(*Client)

// WithHTTPClient overrides the default *http.Client (30s timeout) used for
// requests to the Nacos server. Passing a nil client is a no-op so callers
// can't accidentally disable transport.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// WithAPIVersion pins the Nacos API version, skipping the
// /console/server/state probe performed by Init. Use it when the server
// restricts access to the state endpoint. version must be "v1" or "v3";
// an unsupported value is rejected by NewClient as ErrInvalidAPIVersion.
//
// When the version is pinned, Init is a no-op and GetVersion returns
// ("", nil) since the server version is not probed.
func WithAPIVersion(version string) Option {
	return func(c *Client) { c.apiVersion = version }
}

// NewClient creates a Nacos client targeting urlStr (scheme + host required,
// e.g. "http://localhost:8848"). The returned client is not ready for use
// until Init has been called to detect the server's API version.
//
// Optional client configuration can be supplied via opts, e.g.
//
//	nacos.NewClient(host, user, pass, nacos.WithHTTPClient(myClient))
func NewClient(urlStr, user, password string, opts ...Option) (*Client, error) {
	if !strings.HasSuffix(urlStr, "/") {
		urlStr += "/"
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	c := &Client{
		url:        u,
		user:       user,
		password:   password,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	// An apiVersion pinned via WithAPIVersion must name a known version; an
	// unsupported value would later index api[c.apiVersion][...] and return
	// an empty path, producing confusing empty-endpoint requests.
	if c.apiVersion != "" && api[c.apiVersion] == nil {
		return nil, ErrInvalidAPIVersion
	}
	return c, nil
}

// HTTPClient returns the *http.Client used for requests to the Nacos server.
// The returned pointer is shared with the client; do not mutate its Transport
// concurrently with in-flight requests. Use WithHTTPClient at construction to
// supply a custom client.
func (c *Client) HTTPClient() *http.Client { return c.httpClient }

type Token struct {
	AccessToken string `json:"accessToken"`
	TokenTTL    int64  `json:"tokenTtl"`
	GlobalAdmin bool   `json:"globalAdmin"`
	Username    string `json:"username"`
	ExpiredAt   int64
}

func (t *Token) Expired() bool {
	return time.Now().After(time.Unix(t.ExpiredAt-30, 0))
}

type State struct {
	Version        string `json:"version"`
	StandaloneMode string `json:"standalone_mode"`
	FunctionMode   string `json:"function_mode"`
}

// getVersion probes v3 then v1 and records the first server that reports a
// non-empty version. It is only called by Init, which serializes calls and
// guarantees c.apiVersion == "" on entry, so it does not handle the pinned
// case.
func (c *Client) getVersion(ctx context.Context) error {
	for _, ver := range []string{"v3", "v1"} {
		// fresh state per probe so a partially-decoded prior response
		// cannot leak fields into the next iteration's decode.
		var state State
		if err := c.doRequest(ctx, http.MethodGet, api[ver]["state"], nil, &state); err == nil && state.Version != "" {
			c.apiVersion = ver
			c.state = &state
			return nil
		}
	}
	return fmt.Errorf("unable to get api version")
}

// Init probes the Nacos server once to detect the API version and populate
// State. It must be called — and complete — before any concurrent use of the
// client. It is idempotent: a call after a successful Init is a no-op. A
// failed Init does not mark the client as initialized, so retrying is safe.
// If the API version was pinned via WithAPIVersion, Init is a no-op and no
// /console/server/state request is made.
//
// After Init completes, c.apiVersion and c.state are read-only, so the read
// path (GetToken and all resource methods) needs no synchronization for them.
func (c *Client) Init(ctx context.Context) error {
	c.initMu.Lock()
	defer c.initMu.Unlock()
	if c.apiVersion != "" {
		return nil
	}
	return c.getVersion(ctx)
}

// GetVersion returns the cached Nacos server version. It performs no I/O and
// requires Init to have completed; without it returns ErrNotInitialized.
// When the version was pinned via WithAPIVersion (no probe), it returns
// ("", nil) since the server version is unknown.
func (c *Client) GetVersion(ctx context.Context) (string, error) {
	if c.apiVersion == "" {
		return "", ErrNotInitialized
	}
	if c.state == nil {
		return "", nil
	}
	return c.state.Version, nil
}

func (c *Client) GetToken(ctx context.Context) (string, error) {
	if c.apiVersion == "" {
		return "", ErrNotInitialized
	}
	// fast path: snapshot the token pointer under RLock so it is
	// race-free with concurrent refresh. The pointed-to *Token is never
	// mutated in place after publication (refresh builds a new object),
	// so reading its fields after releasing the lock is safe.
	c.mu.RLock()
	tok := c.token
	c.mu.RUnlock()
	if tok != nil && !tok.Expired() {
		return tok.AccessToken, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// double-check under the write lock
	if c.token != nil && !c.token.Expired() {
		return c.token.AccessToken, nil
	}

	v := url.Values{}
	v.Add("username", c.user)
	v.Add("password", c.password)
	var token Token
	if err := c.doRequest(ctx, http.MethodPost, api[c.apiVersion]["token"], v, &token); err != nil {
		return "", err
	}
	// fully initialize the token before publishing the pointer, so a
	// fast-path reader can never observe a half-written token (e.g.
	// ExpiredAt == 0, which would spuriously read as "expired").
	token.ExpiredAt = time.Now().Unix() + token.TokenTTL
	c.token = &token
	return token.AccessToken, nil
}

func (c *Client) ListNamespace(ctx context.Context) (*NamespaceList, error) {
	token, err := c.GetToken(ctx)
	if err != nil {
		return nil, err
	}
	v := url.Values{}
	v.Add("accessToken", token)
	var nss NamespaceList
	if err := c.doRequest(ctx, http.MethodGet, api[c.apiVersion]["list_ns"], v, &nss); err != nil {
		return nil, err
	}
	return &nss, nil
}

type NsOpts struct {
	Name        string
	Description string
	ID          string
}

func (c *Client) CreateNamespace(ctx context.Context, opts *NsOpts) error {
	token, err := c.GetToken(ctx)
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Add("customNamespaceId", opts.ID)
	v.Add("namespaceName", opts.Name)
	v.Add("namespaceDesc", opts.Description)
	v.Add("accessToken", token)
	return c.doRequest(ctx, http.MethodPost, api[c.apiVersion]["ns"], v, nil)
}

func (c *Client) DeleteNamespace(ctx context.Context, id string) error {
	token, err := c.GetToken(ctx)
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Add("namespaceId", id)
	v.Add("accessToken", token)
	return c.doRequest(ctx, http.MethodDelete, api[c.apiVersion]["ns"], v, nil)
}

func (c *Client) UpdateNamespace(ctx context.Context, opts *NsOpts) error {
	token, err := c.GetToken(ctx)
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Add("namespace", opts.ID)
	v.Add("namespaceId", opts.ID)
	v.Add("namespaceShowName", opts.Name)
	v.Add("namespaceName", opts.Name)
	v.Add("namespaceDesc", opts.Description)
	v.Add("accessToken", token)
	return c.doRequest(ctx, http.MethodPut, api[c.apiVersion]["ns"], v, nil)
}

func (c *Client) CreateOrUpdateNamespace(ctx context.Context, opts *NsOpts) error {
	nsList, err := c.ListNamespace(ctx)
	if err != nil {
		return err
	}
	for _, ns := range nsList.Items {
		if ns.ID == opts.ID {
			return c.UpdateNamespace(ctx, opts)
		}
	}
	return c.CreateNamespace(ctx, opts)
}

func (c *Client) GetNamespace(ctx context.Context, id string) (*Namespace, error) {
	nsList, err := c.ListNamespace(ctx)
	if err != nil {
		return nil, err
	}
	for _, ns := range nsList.Items {
		if ns.ID == id {
			return ns, nil
		}
	}
	return nil, ErrNotFound
}

type GetCfgOpts struct {
	DataID      string
	Group       string
	NamespaceID string
}

var ErrNotFound = errors.New("not found")

// ErrNotInitialized is returned when a method is called before Init has
// successfully detected the server's API version.
var ErrNotInitialized = errors.New("nacos: client not initialized, call Init first")

// ErrInvalidAPIVersion is returned by NewClient when WithAPIVersion is given
// a value that is not a known Nacos API version ("v1" or "v3").
var ErrInvalidAPIVersion = errors.New("nacos: invalid api version")

func (c *Client) GetConfig(ctx context.Context, opts *GetCfgOpts) (*Configuration, error) {
	if opts == nil {
		return nil, errors.New("opts is nil")
	}
	token, err := c.GetToken(ctx)
	if err != nil {
		return nil, err
	}
	v := url.Values{}
	v.Add("dataId", opts.DataID)
	v.Add("group", opts.Group)
	v.Add("groupName", opts.Group)
	v.Add("namespaceId", opts.NamespaceID)
	v.Add("tenant", opts.NamespaceID)
	v.Add("show", "all")
	v.Add("accessToken", token)

	if c.apiVersion == "v1" {
		var v1 Configuration
		if err := c.doRequest(ctx, http.MethodGet, api[c.apiVersion]["cs"], v, &v1); err != nil {
			if err == io.EOF {
				return nil, ErrNotFound
			}
			return nil, err
		}
		return &v1, nil
	}
	var v3 ConfigurationV3
	if err = c.doRequest(ctx, http.MethodGet, api[c.apiVersion]["cs"], v, &v3); err != nil {
		return nil, err
	}
	if v3.Data == nil {
		return nil, ErrNotFound
	}
	return v3.Data, nil

}

type ListCfgOpts struct {
	Application string
	DataID      string
	Group       string
	NamespaceID string
	Tags        string
	PageNumber  int
	PageSize    int
}

func (c *Client) ListConfig(ctx context.Context, opts *ListCfgOpts) (*ConfigurationList, error) {
	token, err := c.GetToken(ctx)
	if err != nil {
		return nil, err
	}
	v := url.Values{}
	v.Add("dataId", opts.DataID)
	v.Add("group", opts.Group)
	v.Add("groupName", opts.Group)
	v.Add("appName", opts.Application)
	v.Add("config_tags", opts.Tags)
	v.Add("configTags", opts.Tags)
	pageNo := opts.PageNumber
	if pageNo == 0 {
		pageNo = 1
	}
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = 10
	}
	v.Add("pageNo", strconv.Itoa(pageNo))
	v.Add("pageSize", strconv.Itoa(pageSize))
	v.Add("tenant", opts.NamespaceID)
	v.Add("namespaceId", opts.NamespaceID)
	v.Add("search", "accurate")
	v.Add("accessToken", token)

	if c.apiVersion == "v1" {
		var v1 ConfigurationList
		if err := c.doRequest(ctx, http.MethodGet, api[c.apiVersion]["list_cs"], v, &v1); err != nil {
			return nil, err
		}
		return &v1, nil
	}
	var v3 ConfigurationListV3
	if err := c.doRequest(ctx, http.MethodGet, api[c.apiVersion]["list_cs"], v, &v3); err != nil {
		return nil, err
	}
	return &v3.Data, nil
}

func (c *Client) ListConfigInNs(ctx context.Context, namespace, group string) (*ConfigurationList, error) {
	nsCs := new(ConfigurationList)
	listOpts := ListCfgOpts{PageNumber: 1, PageSize: 100, Group: group, NamespaceID: namespace}
	for {
		cs, err := c.ListConfig(ctx, &listOpts)
		if err != nil {
			return nil, err
		}
		nsCs.Items = append(nsCs.Items, cs.Items...)
		if cs.PagesAvailable == 0 || cs.PagesAvailable == cs.PageNumber {
			break
		}
		listOpts.PageNumber += 1
	}
	return nsCs, nil
}

func (c *Client) ListAllConfig(ctx context.Context) (*ConfigurationList, error) {
	allCs := new(ConfigurationList)
	nss, err := c.ListNamespace(ctx)
	if err != nil {
		return nil, err
	}
	for _, ns := range nss.Items {
		cs, err := c.ListConfigInNs(ctx, ns.ID, "")
		if err != nil {
			return nil, err
		}
		allCs.Items = append(allCs.Items, cs.Items...)
	}
	return allCs, nil
}

type PublishCfgOpts struct {
	Application string
	Content     string
	DataID      string
	Description string
	Group       string
	NamespaceID string
	Tags        string
	Type        string
}

func (c *Client) PublishConfig(ctx context.Context, opts *PublishCfgOpts) error {
	token, err := c.GetToken(ctx)
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Add("dataId", opts.DataID)
	v.Add("group", opts.Group)
	v.Add("groupName", opts.Group)
	v.Add("content", opts.Content)
	v.Add("type", opts.Type)
	v.Add("tenant", opts.NamespaceID)
	v.Add("namespaceId", opts.NamespaceID)
	v.Add("appName", opts.Application)
	v.Add("desc", opts.Description)
	v.Add("config_tags", opts.Tags)
	v.Add("configTags", opts.Tags)
	v.Add("accessToken", token)
	return c.doRequest(ctx, http.MethodPost, api[c.apiVersion]["cs"], v, nil)
}

type DeleteCfgOpts = GetCfgOpts

func (c *Client) DeleteConfig(ctx context.Context, opts *DeleteCfgOpts) error {
	token, err := c.GetToken(ctx)
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Add("dataId", opts.DataID)
	v.Add("group", opts.Group)
	v.Add("groupName", opts.Group)
	v.Add("tenant", opts.NamespaceID)
	v.Add("namespaceId", opts.NamespaceID)
	v.Add("accessToken", token)

	return c.doRequest(ctx, http.MethodDelete, api[c.apiVersion]["cs"], v, nil)
}

func (c *Client) CreateUser(ctx context.Context, name, password string) error {
	token, err := c.GetToken(ctx)
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Add("username", name)
	v.Add("password", password)
	v.Add("accessToken", token)
	return c.doRequest(ctx, http.MethodPost, api[c.apiVersion]["user"], v, nil)
}

func (c *Client) DeleteUser(ctx context.Context, name string) error {
	token, err := c.GetToken(ctx)
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Add("username", name)
	v.Add("accessToken", token)
	return c.doRequest(ctx, http.MethodDelete, api[c.apiVersion]["user"], v, nil)
}

func (c *Client) ListUser(ctx context.Context) (*UserList, error) {
	if c.apiVersion == "v1" {
		return listResource[UserList](ctx, c, api[c.apiVersion]["list_user"])
	}
	return listResource[UserListV3](ctx, c, api[c.apiVersion]["list_user"])
}

func (c *Client) GetUser(ctx context.Context, name string) (*User, error) {
	users, err := c.ListUser(ctx)
	if err != nil {
		return nil, err
	}

	for _, user := range users.Items {
		if user.Name == name {
			return user, nil
		}
	}
	return nil, ErrNotFound
}

func (c *Client) CreateRole(ctx context.Context, name, username string) error {
	token, err := c.GetToken(ctx)
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Add("username", username)
	v.Add("role", name)
	v.Add("accessToken", token)
	return c.doRequest(ctx, http.MethodPost, api[c.apiVersion]["role"], v, nil)
}

func (c *Client) DeleteRole(ctx context.Context, name, username string) error {
	token, err := c.GetToken(ctx)
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Add("username", username)
	v.Add("role", name)
	v.Add("accessToken", token)
	return c.doRequest(ctx, http.MethodDelete, api[c.apiVersion]["role"], v, nil)
}

func (c *Client) ListRole(ctx context.Context) (*RoleList, error) {
	if c.apiVersion == "v1" {
		return listResource[RoleList](ctx, c, api[c.apiVersion]["list_role"])
	}
	return listResource[RoleListV3](ctx, c, api[c.apiVersion]["list_role"])
}

func (c *Client) GetRole(ctx context.Context, name, username string) (*Role, error) {
	roles, err := c.ListRole(ctx)
	if err != nil {
		return nil, err
	}
	r := Role{Name: name, Username: username}
	if roles.Contains(r) {
		return &r, nil
	}
	return nil, ErrNotFound
}

func (c *Client) CreatePermission(ctx context.Context, role, resource, permission string) error {
	token, err := c.GetToken(ctx)
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Add("action", permission)
	v.Add("resource", resource)
	v.Add("role", role)
	v.Add("accessToken", token)
	return c.doRequest(ctx, http.MethodPost, api[c.apiVersion]["perm"], v, nil)
}

func (c *Client) DeletePermission(ctx context.Context, role, resource, permission string) error {
	token, err := c.GetToken(ctx)
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Add("action", permission)
	v.Add("resource", resource)
	v.Add("role", role)
	v.Add("accessToken", token)
	return c.doRequest(ctx, http.MethodDelete, api[c.apiVersion]["perm"], v, nil)
}

func (c *Client) ListPermission(ctx context.Context) (*PermissionList, error) {
	if c.apiVersion == "v1" {
		return listResource[PermissionList](ctx, c, api[c.apiVersion]["list_perm"])
	}
	return listResource[PermissionListV3](ctx, c, api[c.apiVersion]["list_perm"])
}

func (c *Client) GetPermission(ctx context.Context, role, resource, action string) (*Permission, error) {
	perms, err := c.ListPermission(ctx)
	if err != nil {
		return nil, err
	}
	p := Permission{Role: role, Resource: resource, Action: action}
	if perms.Contains(p) {
		return &p, nil
	}
	return nil, ErrNotFound
}

func listResource[L Paginator[T], T ListTypes](ctx context.Context, c *Client, endpoint string) (*List[T], error) {
	token, err := c.GetToken(ctx)
	if err != nil {
		return nil, err
	}
	all := new(List[T])
	v := url.Values{}
	v.Add("search", "accurate")
	v.Add("accessToken", token)
	v.Add("pageNo", "1")
	v.Add("pageSize", "100")
	for {
		var lst L
		if err := c.doRequest(ctx, http.MethodGet, endpoint, v, &lst); err != nil {
			return nil, err
		}
		all.Items = append(all.Items, lst.All()...)
		if lst.IsEnd() {
			break
		}
		v.Set("pageNo", strconv.Itoa(lst.NextPageNumber()))
	}
	return all, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, values url.Values, v any) error {
	newUrl := c.url.JoinPath(path)
	reqHeaders := make(http.Header)
	var body io.Reader
	if values != nil {
		if method == http.MethodGet || method == http.MethodDelete {
			newUrl.RawQuery = values.Encode()
		} else {
			reqHeaders.Set("Content-Type", "application/x-www-form-urlencoded")
			body = strings.NewReader(values.Encode())
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, newUrl.String(), body)
	if err != nil {
		return err
	}

	maps.Copy(req.Header, reqHeaders)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		// no data or html data
		if len(data) == 0 || data[0] == '<' {
			return NacosErr{Code: resp.StatusCode, URL: newUrl.String()}
		}
		return NacosErr{Code: resp.StatusCode, URL: newUrl.String(), Err: errors.New(string(data))}
	}
	if v != nil {
		err = json.NewDecoder(resp.Body).Decode(v)
	}
	return err

}

type NacosErr struct {
	Code int
	Err  error
	URL  string
}

func (e NacosErr) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%d %s %s", e.Code, e.URL, e.Err.Error())
	}
	return fmt.Sprintf("%d %s", e.Code, e.URL)
}

func (e NacosErr) Unwrap() error {
	return e.Err
}

// Is reports whether the error matches target. A NacosErr carrying an HTTP
// 404 matches ErrNotFound, so callers can uniformly check absence with
// errors.Is(err, nacos.ErrNotFound) regardless of whether the not-found
// result came from the server (404 response) or a client-side absence check
// (e.g. an item missing from a listed page).
func (e NacosErr) Is(target error) bool {
	return target == ErrNotFound && e.Code == http.StatusNotFound
}

func (e NacosErr) IsNotFound() bool {
	return e.Code == http.StatusNotFound
}
