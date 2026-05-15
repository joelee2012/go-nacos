/*
Copyright © 2025 Joe Lee <lj_2005@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
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
	URL        *url.URL
	User       string
	Password   string
	APIVersion string
	HTTPClient *http.Client
	Token      *Token
	State      *State
	mu         sync.Mutex
	detectOnce sync.Once
	detectErr  error
}
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

var api = map[string]map[string]string{
	"v1": {
		"state":        "/v1/console/server/state",
		"token":        "/v1/auth/login",
		"list_ns":      "/v1/console/namespaces",
		"ns":           "/v1/console/namespaces",
		"cs":           "/v1/cs/configs",
		"list_cs":      "/v1/cs/configs",
		"user":         "/v1/auth/users",
		"list_user":    "/v1/auth/users",
		"role":         "/v1/auth/roles",
		"list_role":    "/v1/auth/roles",
		"perm":         "/v1/auth/permissions",
		"list_perm":    "/v1/auth/permissions",
		"service":      "/v1/ns/service",
		"list_service": "/v1/ns/service/list",
	},
	"v3": {
		"state":        "/v3/console/server/state",
		"token":        "/v3/auth/user/login",
		"list_ns":      "/v3/console/core/namespace/list",
		"ns":           "/v3/console/core/namespace",
		"cs":           "/v3/console/cs/config",
		"list_cs":      "/v3/console/cs/config/list",
		"list_user":    "/v3/auth/user/list",
		"user":         "/v3/auth/user",
		"list_role":    "/v3/auth/role/list",
		"role":         "/v3/auth/role",
		"perm":         "/v3/auth/permission",
		"list_perm":    "/v3/auth/permission/list",
		"service":      "/v3/ns/service",
		"list_service": "/v3/ns/service/list",
	},
}

func NewClient(urlStr, user, password string) (*Client, error) {
	if !strings.HasSuffix(urlStr, "/") {
		urlStr += "/"
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	return &Client{
		URL:        u,
		User:       user,
		Password:   password,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) getVersion(ctx context.Context) error {
	var state State
	if c.APIVersion != "" {
		if err := c.doRequest(ctx, http.MethodGet, api[c.APIVersion]["state"], nil, &state); err != nil {
			return err
		}
		c.State = &state
		return nil
	}
	for _, ver := range []string{"v3", "v1"} {
		err := c.doRequest(ctx, http.MethodGet, api[ver]["state"], nil, &state)
		if err == nil && state.Version != "" {
			c.APIVersion = ver
			c.State = &state
			return nil
		}
	}
	return fmt.Errorf("unable to get api version")
}

func (c *Client) GetVersion(ctx context.Context) (string, error) {
	c.detectOnce.Do(func() {
		c.detectErr = c.getVersion(ctx)
	})
	if c.detectErr != nil {
		return "", c.detectErr
	}
	return c.State.Version, nil
}

func (c *Client) GetToken(ctx context.Context) (string, error) {
	if _, err := c.GetVersion(ctx); err != nil {
		return "", err
	}
	if c.Token != nil && !c.Token.Expired() {
		return c.Token.AccessToken, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Token != nil && !c.Token.Expired() {
		return c.Token.AccessToken, nil
	}

	v := url.Values{}
	v.Add("username", c.User)
	v.Add("password", c.Password)
	var token Token
	err := c.doRequest(ctx, http.MethodPost, api[c.APIVersion]["token"], v, &token)
	if err != nil {
		return "", err
	}
	c.Token = &token
	c.Token.ExpiredAt = time.Now().Unix() + c.Token.TokenTTL
	return c.Token.AccessToken, nil
}

func (c *Client) ListNamespace(ctx context.Context) (*NamespaceList, error) {
	token, err := c.GetToken(ctx)
	if err != nil {
		return nil, err
	}
	v := url.Values{}
	v.Add("accessToken", token)
	var nss NamespaceList
	if err := c.doRequest(ctx, http.MethodGet, api[c.APIVersion]["list_ns"], v, &nss); err != nil {
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
	return c.doRequest(ctx, http.MethodPost, api[c.APIVersion]["ns"], v, nil)
}

func (c *Client) DeleteNamespace(ctx context.Context, id string) error {
	token, err := c.GetToken(ctx)
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Add("namespaceId", id)
	v.Add("accessToken", token)
	return c.doRequest(ctx, http.MethodDelete, api[c.APIVersion]["ns"], v, nil)
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
	return c.doRequest(ctx, http.MethodPut, api[c.APIVersion]["ns"], v, nil)
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

	if c.APIVersion == "v1" {
		var v1 Configuration
		if err := c.doRequest(ctx, http.MethodGet, api[c.APIVersion]["cs"], v, &v1); err != nil {
			if err == io.EOF {
				return nil, ErrNotFound
			}
			return nil, err
		}
		return &v1, nil
	}
	var v3 ConfigurationV3
	if err = c.doRequest(ctx, http.MethodGet, api[c.APIVersion]["cs"], v, &v3); err != nil {
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
	if opts.PageNumber == 0 {
		opts.PageNumber = 1
	}
	if opts.PageSize == 0 {
		opts.PageSize = 10
	}
	v.Add("pageNo", strconv.Itoa(opts.PageNumber))
	v.Add("pageSize", strconv.Itoa(opts.PageSize))
	v.Add("tenant", opts.NamespaceID)
	v.Add("namespaceId", opts.NamespaceID)
	v.Add("search", "accurate")
	v.Add("accessToken", token)

	if c.APIVersion == "v1" {
		var v1 ConfigurationList
		if err := c.doRequest(ctx, http.MethodGet, api[c.APIVersion]["list_cs"], v, &v1); err != nil {
			return nil, err
		}
		return &v1, nil
	}
	var v3 ConfigurationListV3
	if err := c.doRequest(ctx, http.MethodGet, api[c.APIVersion]["list_cs"], v, &v3); err != nil {
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

type CreateCfgOpts struct {
	Application string
	Content     string
	DataID      string
	Description string
	Group       string
	NamespaceID string
	Tags        string
	Type        string
}

func (c *Client) CreateConfig(ctx context.Context, opts *CreateCfgOpts) error {
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
	return c.doRequest(ctx, http.MethodPost, api[c.APIVersion]["cs"], v, nil)
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

	return c.doRequest(ctx, http.MethodDelete, api[c.APIVersion]["cs"], v, nil)
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
	return c.doRequest(ctx, http.MethodPost, api[c.APIVersion]["user"], v, nil)
}

func (c *Client) DeleteUser(ctx context.Context, name string) error {
	token, err := c.GetToken(ctx)
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Add("username", name)
	v.Add("accessToken", token)
	return c.doRequest(ctx, http.MethodDelete, api[c.APIVersion]["user"], v, nil)
}

func (c *Client) ListUser(ctx context.Context) (*UserList, error) {
	if c.APIVersion == "v1" {
		return listResource[UserList](ctx, c, api[c.APIVersion]["list_user"])
	}
	return listResource[UserListV3](ctx, c, api[c.APIVersion]["list_user"])
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
	return c.doRequest(ctx, http.MethodPost, api[c.APIVersion]["role"], v, nil)
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
	return c.doRequest(ctx, http.MethodDelete, api[c.APIVersion]["role"], v, nil)
}

func (c *Client) ListRole(ctx context.Context) (*RoleList, error) {
	if c.APIVersion == "v1" {
		return listResource[RoleList](ctx, c, api[c.APIVersion]["list_role"])
	}
	return listResource[RoleListV3](ctx, c, api[c.APIVersion]["list_role"])
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
	return c.doRequest(ctx, http.MethodPost, api[c.APIVersion]["perm"], v, nil)
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
	return c.doRequest(ctx, http.MethodDelete, api[c.APIVersion]["perm"], v, nil)
}

func (c *Client) ListPermission(ctx context.Context) (*PermissionList, error) {
	if c.APIVersion == "v1" {
		return listResource[PermissionList](ctx, c, api[c.APIVersion]["list_perm"])
	}
	return listResource[PermissionListV3](ctx, c, api[c.APIVersion]["list_perm"])
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
	newUrl := c.URL.JoinPath(path)
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

	resp, err := c.HTTPClient.Do(req)
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
func (e NacosErr) IsNotFound() bool {
	return e.Code == http.StatusNotFound
}
