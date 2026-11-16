package cloud

import "testing"

func TestAuthOptionsFromDevstackOpenrc(t *testing.T) {
	// Variables exported by /opt/stack/devstack/openrc admin admin
	t.Setenv("OS_AUTH_URL", "http://192.168.153.50/identity")
	t.Setenv("OS_USERNAME", "admin")
	t.Setenv("OS_PASSWORD", "secret")
	t.Setenv("OS_PROJECT_NAME", "admin")
	t.Setenv("OS_USER_DOMAIN_ID", "default")
	t.Setenv("OS_PROJECT_DOMAIN_ID", "default")

	ao, err := authOptionsFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if ao.DomainID != "default" || ao.Scope == nil || ao.Scope.ProjectName != "admin" || ao.Scope.DomainID != "default" {
		t.Fatalf("unexpected options: %+v scope=%+v", ao, ao.Scope)
	}
	if _, err := ao.ToTokenV3ScopeMap(); err != nil {
		t.Fatalf("scope rejected: %v", err)
	}
}

func TestAuthOptionsMissingURL(t *testing.T) {
	t.Setenv("OS_AUTH_URL", "")
	if _, err := authOptionsFromEnv(); err == nil {
		t.Fatal("expected error")
	}
}
