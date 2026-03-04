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
	fmt.Printf("Debug - ACC env: '%s'\n", os.Getenv("ACC"))
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

	testCases := []struct {
		name     string
		host     string
		user     string
		password string
		wantErr  bool
	}{
		{
			name:     "valid credentials",
			host:     os.Getenv("NACOS_HOST"),
			user:     os.Getenv("NACOS_USERNAME"),
			password: os.Getenv("NACOS_PASSWORD"),
			wantErr:  false,
		},
		{
			name:     "empty host",
			host:     "",
			user:     "user",
			password: "pass",
			wantErr:  true,
		},
		{
			name:     "empty user",
			host:     "host",
			user:     "",
			password: "pass",
			wantErr:  true,
		},
		{
			name:     "empty password",
			host:     "host",
			user:     "user",
			password: "",
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Backup and restore env vars
			oldHost := os.Getenv("NACOS_HOST")
			oldUser := os.Getenv("NACOS_USERNAME")
			oldPass := os.Getenv("NACOS_PASSWORD")
			defer func() {
				os.Setenv("NACOS_HOST", oldHost)
				os.Setenv("NACOS_USERNAME", oldUser)
				os.Setenv("NACOS_PASSWORD", oldPass)
			}()

			// Set test case env vars
			os.Setenv("NACOS_HOST", tc.host)
			os.Setenv("NACOS_USERNAME", tc.user)
			os.Setenv("NACOS_PASSWORD", tc.password)

			client := createTestClient(t)
			if tc.wantErr {
				assert.Empty(t, client.URL)
			} else {
				assert.NotEmpty(t, client.URL)
				assert.NotEmpty(t, client.User)
				assert.NotEmpty(t, client.Password)
			}
		})
	}
}

func createTestNamespace(t *testing.T, client *nacos.Client, name string) string {
	ctx := context.Background()
	nsID := randomID()
	err := client.CreateNamespace(ctx, &nacos.CreateNsOpts{
		Name:        name,
		Description: "Test namespace",
		ID:          nsID,
	})
	assert.NoError(t, err)
	t.Cleanup(func() {
		_ = client.DeleteNamespace(ctx, nsID, false)
	})
	return nsID
}

func TestAccNamespaceCRUD(t *testing.T) {
	skipIfNotAcc(t)

	client := createTestClient(t)
	ctx := context.Background()
	initialName := "Test Namespace"
	updatedName := "Updated Namespace"

	// Create and verify
	nsID := createTestNamespace(t, client, initialName)
	ns, err := client.GetNamespace(ctx, nsID)
	assert.NoError(t, err)
	assert.Equal(t, nsID, ns.ID)
	assert.Equal(t, initialName, ns.Name)

	// Update and verify
	err = client.UpdateNamespace(ctx, &nacos.CreateNsOpts{
		ID:   nsID,
		Name: updatedName,
	})
	assert.NoError(t, err)
	ns, err = client.GetNamespace(ctx, nsID)
	assert.NoError(t, err)
	assert.Equal(t, updatedName, ns.Name)
}

func createTestConfig(t *testing.T, client *nacos.Client, nsID string, content string) string {
	ctx := context.Background()
	cfgDataID := "test-config" + randomID()
	err := client.CreateConfig(ctx, &nacos.CreateCfgOpts{
		DataID:      cfgDataID,
		Group:       "DEFAULT_GROUP",
		NamespaceID: nsID,
		Content:     content,
		Type:        "properties",
	})
	assert.NoError(t, err)
	t.Cleanup(func() {
		_ = client.DeleteConfig(ctx, &nacos.DeleteCfgOpts{
			DataID:      cfgDataID,
			Group:       "DEFAULT_GROUP",
			NamespaceID: nsID,
		})
	})
	return cfgDataID
}

func TestAccConfigCRUD(t *testing.T) {
	skipIfNotAcc(t)

	client := createTestClient(t)
	ctx := context.Background()

	// Setup test namespace
	nsID := createTestNamespace(t, client, "Config Test NS")

	// Test config operations
	initialContent := "test.key=test.value"
	updatedContent := "test.key=updated.value"

	// Create and verify
	cfgDataID := createTestConfig(t, client, nsID, initialContent)
	cfg, err := client.GetConfig(ctx, &nacos.GetCfgOpts{
		DataID:      cfgDataID,
		Group:       "DEFAULT_GROUP",
		NamespaceID: nsID,
	})
	assert.NoError(t, err)
	assert.Equal(t, initialContent, cfg.Content)

	// Update and verify
	err = client.CreateConfig(ctx, &nacos.CreateCfgOpts{
		DataID:      cfgDataID,
		Group:       "DEFAULT_GROUP",
		NamespaceID: nsID,
		Content:     updatedContent,
		Type:        "properties",
	})
	assert.NoError(t, err)
	cfg, err = client.GetConfig(ctx, &nacos.GetCfgOpts{
		DataID:      cfgDataID,
		Group:       "DEFAULT_GROUP",
		NamespaceID: nsID,
	})
	assert.NoError(t, err)
	assert.Equal(t, updatedContent, cfg.Content)
}
