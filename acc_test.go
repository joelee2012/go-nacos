package nacos_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joelee2012/go-nacos"
	"github.com/stretchr/testify/assert"
)

func skipIfNotAcc(t *testing.T) {
	if os.Getenv("ACC") != "true" {
		t.Skip("Skipping acceptance test (ACC=true required)")
	}
}

func createTestClient(t *testing.T) *nacos.Client {
	host := os.Getenv("NACOS_HOST")
	user := os.Getenv("NACOS_USERNAME")
	password := os.Getenv("NACOS_PASSWORD")

	if host == "" || user == "" || password == "" {
		t.Fatal("NACOS_HOST, NACOS_USERNAME and NACOS_PASSWORD must be set for acceptance tests")
	}

	// Ensure URL has https scheme if not specified
	if !strings.HasPrefix(host, "http") {
		host = "https://" + host
	}

	client := nacos.NewClient(host, user, password)

	// Verify client can connect
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.GetVersion(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize client: %v", err)
	}

	return client
}

func randomID() string {
	return fmt.Sprintf("test-%d", time.Now().UnixNano())
}

func TestAccClientInitialization(t *testing.T) {
	skipIfNotAcc(t)

	client := createTestClient(t)
	assert.NotEmpty(t, client.URL)
	assert.NotEmpty(t, client.User)
	assert.NotEmpty(t, client.Password)
}

func TestAccNamespaceCRUD(t *testing.T) {
	skipIfNotAcc(t)

	client := createTestClient(t)
	ctx := context.Background()

	// Create
	nsID := randomID()
	opts := &nacos.CreateNsOpts{
		Name:        "Test Namespace",
		Description: "Acceptance test namespace",
		ID:          nsID,
	}
	err := client.CreateNamespace(ctx, opts)
	assert.NoError(t, err)

	// Read
	ns, err := client.GetNamespace(ctx, nsID)
	assert.NoError(t, err)
	assert.Equal(t, nsID, ns.ID)
	assert.Equal(t, "Test Namespace", ns.Name)

	// Update
	opts.Name = "Updated Namespace"
	err = client.UpdateNamespace(ctx, opts)
	assert.NoError(t, err)

	// Verify update
	ns, err = client.GetNamespace(ctx, nsID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Namespace", ns.Name)

	// Cleanup (will fail if namespace contains configs)
	err = client.DeleteNamespace(ctx, nsID, false)
	assert.NoError(t, err)

	// Verify deletion
	_, err = client.GetNamespace(ctx, nsID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404 Not Found")
}

func TestAccConfigCRUD(t *testing.T) {
	skipIfNotAcc(t)

	client := createTestClient(t)
	ctx := context.Background()

	// Setup test namespace
	nsID := randomID()
	err := client.CreateNamespace(ctx, &nacos.CreateNsOpts{
		Name: "Config Test NS",
		ID:   nsID,
	})
	assert.NoError(t, err)
	defer func() {
		_ = client.DeleteNamespace(ctx, nsID, false)
	}()

	// Create config
	cfgDataID := "test-config" + randomID()
	cfgContent := "test.key=test.value"
	err = client.CreateConfig(ctx, &nacos.CreateCfgOpts{
		DataID:      cfgDataID,
		Group:       "DEFAULT_GROUP",
		NamespaceID: nsID,
		Content:     cfgContent,
		Type:        "properties",
	})
	assert.NoError(t, err)

	// Get config
	cfg, err := client.GetConfig(ctx, &nacos.GetCfgOpts{
		DataID:      cfgDataID,
		Group:       "DEFAULT_GROUP",
		NamespaceID: nsID,
	})
	assert.NoError(t, err)
	assert.Equal(t, cfgContent, cfg.Content)

	// Update config
	updatedContent := "test.key=updated.value"
	err = client.CreateConfig(ctx, &nacos.CreateCfgOpts{
		DataID:      cfgDataID,
		Group:       "DEFAULT_GROUP",
		NamespaceID: nsID,
		Content:     updatedContent,
		Type:        "properties",
	})
	assert.NoError(t, err)

	// Verify update
	cfg, err = client.GetConfig(ctx, &nacos.GetCfgOpts{
		DataID:      cfgDataID,
		Group:       "DEFAULT_GROUP",
		NamespaceID: nsID,
	})
	assert.NoError(t, err)
	assert.Equal(t, updatedContent, cfg.Content)

	// Delete config
	err = client.DeleteConfig(ctx, &nacos.DeleteCfgOpts{
		DataID:      cfgDataID,
		Group:       "DEFAULT_GROUP",
		NamespaceID: nsID,
	})
	assert.NoError(t, err)

	// Verify deletion
	_, err = client.GetConfig(ctx, &nacos.GetCfgOpts{
		DataID:      cfgDataID,
		Group:       "DEFAULT_GROUP",
		NamespaceID: nsID,
	})

	assert.Error(t, err)
}

func TestAccUserCRUD(t *testing.T) {
	skipIfNotAcc(t)

	client := createTestClient(t)
	ctx := context.Background()

	// Create user
	username := "test-user-" + randomID()
	err := client.CreateUser(ctx, username, "test-password")
	assert.NoError(t, err)

	// Get user
	user, err := client.GetUser(ctx, username)
	assert.NoError(t, err)
	assert.Equal(t, username, user.Name)

	// Delete user
	err = client.DeleteUser(ctx, username)
	assert.NoError(t, err)

	// Verify deletion
	_, err = client.GetUser(ctx, username)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404 Not Found")
}

func TestAccRoleCRUD(t *testing.T) {
	skipIfNotAcc(t)

	client := createTestClient(t)
	ctx := context.Background()

	// Create test user
	username := "test-user-" + randomID()
	err := client.CreateUser(ctx, username, "test-password")
	assert.NoError(t, err)
	defer func() {
		_ = client.DeleteUser(ctx, username)
	}()

	// Create role
	roleName := "TEST_ROLE"
	err = client.CreateRole(ctx, roleName, username)
	assert.NoError(t, err)

	// Get role
	role, err := client.GetRole(ctx, roleName, username)
	assert.NoError(t, err)
	assert.Equal(t, roleName, role.Name)
	assert.Equal(t, username, role.Username)

	// Delete role
	err = client.DeleteRole(ctx, roleName, username)
	assert.NoError(t, err)

	// Verify deletion
	_, err = client.GetRole(ctx, roleName, username)
	assert.Error(t, err)
}

func TestAccPermissionCRUD(t *testing.T) {
	skipIfNotAcc(t)

	client := createTestClient(t)
	ctx := context.Background()

	// Create test user and role
	username := "test-user-" + randomID()
	err := client.CreateUser(ctx, username, "test-password")
	assert.NoError(t, err)
	defer func() {
		_ = client.DeleteUser(ctx, username)
	}()

	roleName := "TEST_ROLE"
	err = client.CreateRole(ctx, roleName, username)
	assert.NoError(t, err)
	defer func() {
		_ = client.DeleteRole(ctx, roleName, username)
	}()

	// Create permission
	resource := "test:resource:*"
	action := "rw"
	err = client.CreatePermission(ctx, roleName, resource, action)
	assert.NoError(t, err)

	// Get permission
	perm, err := client.GetPermission(ctx, roleName, resource, action)
	assert.NoError(t, err)
	assert.Equal(t, roleName, perm.Role)
	assert.Equal(t, resource, perm.Resource)
	assert.Equal(t, action, perm.Action)

	// Delete permission
	err = client.DeletePermission(ctx, roleName, resource, action)
	assert.NoError(t, err)

	// Verify deletion
	_, err = client.GetPermission(ctx, roleName, resource, action)
	assert.Error(t, err)
}
