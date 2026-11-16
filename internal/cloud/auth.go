package cloud

import (
	"errors"
	"os"

	"github.com/gophercloud/gophercloud/v2"
)

// authOptionsFromEnv reads a standard openrc (Keystone v3), including
// OS_USER_DOMAIN_* and OS_PROJECT_DOMAIN_*, which gophercloud's
// openstack.AuthOptionsFromEnv ignores (DevStack's openrc only sets those).
func authOptionsFromEnv() (gophercloud.AuthOptions, error) {
	env := func(keys ...string) string {
		for _, k := range keys {
			if v := os.Getenv(k); v != "" {
				return v
			}
		}
		return ""
	}

	ao := gophercloud.AuthOptions{
		IdentityEndpoint: env("OS_AUTH_URL"),
		Username:         env("OS_USERNAME"),
		UserID:           env("OS_USER_ID", "OS_USERID"),
		Password:         env("OS_PASSWORD"),
		DomainID:         env("OS_USER_DOMAIN_ID", "OS_DOMAIN_ID"),
		DomainName:       env("OS_USER_DOMAIN_NAME", "OS_DOMAIN_NAME"),
		AllowReauth:      true,
	}
	switch {
	case ao.IdentityEndpoint == "":
		return ao, errors.New("OS_AUTH_URL is not set (source your openrc)")
	case ao.Username == "" && ao.UserID == "":
		return ao, errors.New("OS_USERNAME or OS_USER_ID is required")
	case ao.Password == "":
		return ao, errors.New("OS_PASSWORD is required")
	case ao.UserID == "" && ao.DomainID == "" && ao.DomainName == "":
		ao.DomainID = "default"
	}

	if id := env("OS_PROJECT_ID", "OS_TENANT_ID"); id != "" {
		ao.Scope = &gophercloud.AuthScope{ProjectID: id}
	} else if name := env("OS_PROJECT_NAME", "OS_TENANT_NAME"); name != "" {
		s := &gophercloud.AuthScope{
			ProjectName: name,
			DomainID:    env("OS_PROJECT_DOMAIN_ID"),
			DomainName:  env("OS_PROJECT_DOMAIN_NAME"),
		}
		if s.DomainID == "" && s.DomainName == "" {
			s.DomainID, s.DomainName = ao.DomainID, ao.DomainName
		}
		ao.Scope = s
	}
	return ao, nil
}
