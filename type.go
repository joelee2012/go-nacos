package nacos

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

type NamespaceList struct {
	// Code    int         `json:"code,omitempty"`
	// Message string      `json:"message,omitempty"`
	Items []*Namespace `json:"data"`
}

type Namespace struct {
	ID          string `json:"namespace"`
	Name        string `json:"namespaceShowName"`
	Description string `json:"namespaceDesc"`
	Quota       int    `json:"quota,omitempty"`
	ConfigCount int    `json:"configCount,omitempty"`
	Type        int    `json:"type,omitempty"`
}

type Configuration struct {
	ID               string `json:"id"`
	DataID           string `json:"dataId"`
	Group            string `json:"group"`
	GroupName        string `json:"groupName"`
	Content          string `json:"content"`
	Tenant           string `json:"tenant"`
	NamespaceID      string `json:"namespaceId"`
	Type             string `json:"type"`
	Md5              string `json:"md5,omitempty"`
	EncryptedDataKey string `json:"encryptedDataKey,omitempty"`
	Application      string `json:"appName,omitempty"`
	CreateTime       int64  `json:"createTime,omitempty"`
	ModifyTime       int64  `json:"modifyTime,omitempty"`
	Description      string `json:"desc,omitempty"`
	Tags             string `json:"configTags,omitempty"`
}

type ConfigurationV3 struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    *Configuration `json:"data"`
}

func (c *Configuration) GetGroup() string {
	if c.Group != "" {
		return c.Group
	}
	return c.GroupName
}

func (c *Configuration) GetNamespace() string {
	if c.Tenant != "" {
		return c.Tenant
	}
	return c.NamespaceID
}

type User struct {
	Name     string `json:"username"`
	Password string `json:"password"`
}

type Role struct {
	Name     string `json:"role"`
	Username string `json:"username"`
}

type Permission struct {
	Role     string `json:"role"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

type ListTypes interface {
	User | Role | Permission | Configuration
}

type List[T ListTypes] struct {
	TotalCount     int  `json:"totalCount,omitempty"`
	PageNumber     int  `json:"pageNumber,omitempty"`
	PagesAvailable int  `json:"pagesAvailable,omitempty"`
	Items          []*T `json:"pageItems"`
}

func (lst *List[T]) Contains(other T) bool {
	for _, it := range lst.Items {
		if *it == other {
			return true
		}
	}
	return false
}

func (lst List[T]) NextPageNumber() int {
	return lst.PageNumber + 1
}

func (lst List[T]) IsEnd() bool {
	return lst.PagesAvailable == 0 || lst.PagesAvailable == lst.PageNumber
}

func (lst List[T]) All() []*T {
	return lst.Items
}

type ConfigurationList = List[Configuration]
type PermissionList = List[Permission]
type RoleList = List[Role]
type UserList = List[User]

type ListV3[T ListTypes] struct {
	Code    int     `json:"code"`
	Message string  `json:"message"`
	Data    List[T] `json:"data"`
}

func (lst ListV3[T]) All() []*T {
	return lst.Data.All()
}

func (lst ListV3[T]) NextPageNumber() int {
	return lst.Data.NextPageNumber()
}

func (lst ListV3[T]) IsEnd() bool {
	return lst.Data.IsEnd()
}

type ConfigurationListV3 = ListV3[Configuration]
type PermissionListV3 = ListV3[Permission]
type RoleListV3 = ListV3[Role]
type UserListV3 = ListV3[User]

type Paginator[T any] interface {
	All() []*T
	NextPageNumber() int
	IsEnd() bool
}
