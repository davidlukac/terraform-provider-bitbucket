package bitbucket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// TestProjectWriteBodyJSON validates the core premise of the projectWriteBody workaround:
// marshaling must NOT include created_on, updated_on, or type fields, which
// bitbucket-go-client v0.11.0 injects when using bitbucket.Project directly.
// See: https://github.com/DrFaust92/bitbucket-go-client/issues/41
func TestProjectWriteBodyJSON(t *testing.T) {
	t.Run("empty struct omits all fields", func(t *testing.T) {
		body := &projectWriteBody{}
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		got := string(data)
		for _, banned := range []string{"created_on", "updated_on", "type"} {
			if strings.Contains(got, banned) {
				t.Errorf("field %q must not appear in marshaled output: %s", banned, got)
			}
		}
	})

	t.Run("populated struct contains only writable fields", func(t *testing.T) {
		body := &projectWriteBody{
			Name:        "My Project",
			Key:         "MYPROJ",
			IsPrivate:   true,
			Description: "test description",
		}
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		got := string(data)

		// Must not include read-only fields from bitbucket.Project
		for _, banned := range []string{"created_on", "updated_on", "type"} {
			if strings.Contains(got, banned) {
				t.Errorf("field %q must not appear in marshaled output: %s", banned, got)
			}
		}

		// Must include the expected writable fields
		for _, want := range []string{`"name"`, `"key"`, `"is_private"`, `"description"`} {
			if !strings.Contains(got, want) {
				t.Errorf("field %s missing from marshaled output: %s", want, got)
			}
		}
	})

	t.Run("is_private false is always serialized", func(t *testing.T) {
		// is_private has no omitempty: false must be sent explicitly on PUT so that
		// Bitbucket actually sets the project to public. Without this, a PUT body missing
		// is_private preserves the existing value, causing a perpetual diff when the user
		// changes is_private from true to false.
		body := &projectWriteBody{Name: "Test", Key: "TEST", IsPrivate: false}
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		got := string(data)
		if !strings.Contains(got, `"is_private"`) {
			t.Errorf("is_private must always be serialized even when false: %s", got)
		}
		if !strings.Contains(got, `"is_private":false`) {
			t.Errorf("is_private=false must serialize as false, got: %s", got)
		}
	})
}

func TestAccBitbucketProject_basic(t *testing.T) {
	resourceName := "bitbucket_project.test"
	testTeam := os.Getenv("BITBUCKET_TEAM")
	rName := acctest.RandomWithPrefix("tf-test")

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckBitbucketProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBitbucketProjectConfig(testTeam, rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBitbucketProjectExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "has_publicly_visible_repos", "false"),
					resource.TestCheckResourceAttr(resourceName, "key", "AAAAAA"),
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "owner", testTeam),
					resource.TestCheckResourceAttr(resourceName, "description", ""),
					resource.TestCheckResourceAttr(resourceName, "is_private", "true"),
					resource.TestCheckResourceAttrSet(resourceName, "uuid"),
					resource.TestCheckResourceAttr(resourceName, "link.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "link.0.avatar.#", "1"),
					resource.TestCheckResourceAttrSet(resourceName, "link.0.avatar.0.href"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccBitbucketProjectDescConfig(testTeam, rName, rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBitbucketProjectExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "has_publicly_visible_repos", "false"),
					resource.TestCheckResourceAttr(resourceName, "key", "AAAAAA"),
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "owner", testTeam),
					resource.TestCheckResourceAttr(resourceName, "description", rName),
					resource.TestCheckResourceAttr(resourceName, "is_private", "true"),
					resource.TestCheckResourceAttrSet(resourceName, "uuid"),
					resource.TestCheckResourceAttr(resourceName, "link.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "link.0.avatar.#", "1"),
					resource.TestCheckResourceAttrSet(resourceName, "link.0.avatar.0.href"),
				),
			},
		},
	})
}

func TestAccBitbucketProject_avatar(t *testing.T) {
	resourceName := "bitbucket_project.test"
	testTeam := os.Getenv("BITBUCKET_TEAM")
	rName := acctest.RandomWithPrefix("tf-test")

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckBitbucketProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBitbucketProjectAvatarConfig(testTeam, rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBitbucketProjectExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "link.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "link.0.avatar.#", "1"),
					resource.TestCheckResourceAttrSet(resourceName, "link.0.avatar.0.href"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccBitbucketProjectConfig(team, rName string) string {
	return fmt.Sprintf(`
resource "bitbucket_project" "test" {
  owner = %[1]q
  name  = %[2]q
  key   = "AAAAAA"
}
`, team, rName)
}

func testAccBitbucketProjectDescConfig(team, rName, desc string) string {
	return fmt.Sprintf(`
resource "bitbucket_project" "test" {
  owner       = %[1]q
  name        = %[2]q
  key         = "AAAAAA"
  description = %[3]q
}
`, team, rName, desc)
}

func testAccBitbucketProjectAvatarConfig(team, rName string) string {
	return fmt.Sprintf(`
resource "bitbucket_project" "test" {
  owner = %[1]q
  name  = %[2]q
  key   = "BBBBB"

  link {
    avatar {
      href = "https://d301sr5gafysq2.cloudfront.net/dfb18959be9c/img/repo-avatars/python.png"
	}
  }
}
`, team, rName)
}

func testAccCheckBitbucketProjectDestroy(s *terraform.State) error {
	client := testAccProvider.Meta().(Clients).genClient
	projectApi := client.ApiClient.ProjectsApi

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "bitbucket_project" {
			continue
		}

		_, res, err := projectApi.WorkspacesWorkspaceProjectsProjectKeyGet(client.AuthContext,
			rs.Primary.Attributes["key"], rs.Primary.Attributes["owner"])

		if err == nil {
			return fmt.Errorf("The resource was found should have errored")
		}

		if res.StatusCode != http.StatusNotFound {
			return fmt.Errorf("Project still exists")
		}
	}
	return nil
}

func testAccCheckBitbucketProjectExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found %s", n)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("No project ID is set")
		}
		return nil
	}
}
