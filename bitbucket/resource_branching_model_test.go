package bitbucket

import (
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// --- Unit tests for flattenBranchTypes ---

func TestFlattenBranchTypes(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name    string
		input   []*BranchType
		wantLen int
		wantNil bool
	}{
		{
			name:    "nil slice",
			input:   nil,
			wantNil: true,
		},
		{
			name:    "empty slice",
			input:   []*BranchType{},
			wantNil: true,
		},
		{
			name: "nil element skipped",
			input: []*BranchType{
				nil,
				{Kind: "feature", Prefix: "feat/", Enabled: &trueVal},
			},
			wantLen: 1,
		},
		{
			name: "nil Enabled pointer yields false",
			input: []*BranchType{
				{Kind: "bugfix", Prefix: "bug/", Enabled: nil},
			},
			wantLen: 1,
		},
		{
			name: "Enabled true",
			input: []*BranchType{
				{Kind: "hotfix", Prefix: "hf/", Enabled: &trueVal},
			},
			wantLen: 1,
		},
		{
			name: "Enabled false",
			input: []*BranchType{
				{Kind: "release", Prefix: "rel/", Enabled: &falseVal},
			},
			wantLen: 1,
		},
		{
			name: "multiple entries",
			input: []*BranchType{
				{Kind: "feature", Prefix: "feat/", Enabled: &trueVal},
				{Kind: "bugfix", Prefix: "bug/", Enabled: &falseVal},
			},
			wantLen: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := flattenBranchTypes(tc.input)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if len(got) != tc.wantLen {
				t.Fatalf("expected len %d, got %d: %v", tc.wantLen, len(got), got)
			}
			// Verify each element has the required keys and bool (not *bool) for enabled.
			for _, raw := range got {
				m, ok := raw.(map[string]interface{})
				if !ok {
					t.Fatalf("element is not map[string]interface{}: %T", raw)
				}
				if _, ok := m["kind"].(string); !ok {
					t.Errorf("kind is not a string: %T", m["kind"])
				}
				if _, ok := m["prefix"].(string); !ok {
					t.Errorf("prefix is not a string: %T", m["prefix"])
				}
				// enabled must be bool, not *bool — this is what we fixed.
				if _, ok := m["enabled"].(bool); !ok {
					t.Errorf("enabled is not bool (got %T), d.Set would silently fail", m["enabled"])
				}
			}
		})
	}
}

// --- Unit tests for flattenBranchModel ---

func TestFlattenBranchModel(t *testing.T) {
	name := "main"

	t.Run("nil input returns empty slice", func(t *testing.T) {
		got := flattenBranchModel(nil, "development")
		if len(got) != 0 {
			t.Fatalf("expected empty, got %v", got)
		}
	})

	t.Run("development type has no enabled key", func(t *testing.T) {
		bm := &BranchModel{UseMainbranch: true, BranchDoesNotExist: false, IsValid: true, Name: &name}
		got := flattenBranchModel(bm, "development")
		if len(got) != 1 {
			t.Fatalf("expected 1 element, got %d", len(got))
		}
		m := got[0].(map[string]interface{})
		if _, has := m["enabled"]; has {
			t.Error("development flattenBranchModel should not include 'enabled' key")
		}
		if m["use_mainbranch"] != true {
			t.Errorf("use_mainbranch: got %v, want true", m["use_mainbranch"])
		}
	})

	t.Run("production type includes enabled key", func(t *testing.T) {
		bm := &BranchModel{UseMainbranch: true, Enabled: true, IsValid: true, Name: &name}
		got := flattenBranchModel(bm, "production")
		if len(got) != 1 {
			t.Fatalf("expected 1 element, got %d", len(got))
		}
		m := got[0].(map[string]interface{})
		if _, has := m["enabled"]; !has {
			t.Error("production flattenBranchModel must include 'enabled' key")
		}
		if m["enabled"] != true {
			t.Errorf("enabled: got %v, want true", m["enabled"])
		}
	})
}

func TestAccBitbucketBranchingModel_basic(t *testing.T) {
	var branchRestriction BranchingModel
	rName := acctest.RandomWithPrefix("tf-test")
	testUser := os.Getenv("BITBUCKET_TEAM")
	resourceName := "bitbucket_branching_model.test"

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckBitbucketBranchingModelDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBitbucketBranchingModelConfig(testUser, rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBitbucketBranchingModelExists(resourceName, &branchRestriction),
					resource.TestCheckResourceAttrPair(resourceName, "repository", "bitbucket_repository.test", "name"),
					resource.TestCheckResourceAttr(resourceName, "development.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "development.0.use_mainbranch", "true"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// Verify no perpetual diff after apply + import.
				Config:             testAccBitbucketBranchingModelConfig(testUser, rName),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func TestAccBitbucketBranchingModel_production(t *testing.T) {
	var branchRestriction BranchingModel
	rName := acctest.RandomWithPrefix("tf-test")
	testUser := os.Getenv("BITBUCKET_TEAM")
	resourceName := "bitbucket_branching_model.test"

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckBitbucketBranchingModelDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBitbucketBranchingModelProdConfig(testUser, rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBitbucketBranchingModelExists(resourceName, &branchRestriction),
					resource.TestCheckResourceAttrPair(resourceName, "repository", "bitbucket_repository.test", "name"),
					resource.TestCheckResourceAttr(resourceName, "development.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "development.0.use_mainbranch", "true"),
					resource.TestCheckResourceAttr(resourceName, "production.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "production.0.use_mainbranch", "true"),
					resource.TestCheckResourceAttr(resourceName, "production.0.enabled", "true"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// Verify no perpetual diff after apply + import.
				Config:             testAccBitbucketBranchingModelProdConfig(testUser, rName),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func TestAccBitbucketBranchingModel_branchTypes(t *testing.T) {
	var branchRestriction BranchingModel
	rName := acctest.RandomWithPrefix("tf-test")
	testUser := os.Getenv("BITBUCKET_TEAM")
	resourceName := "bitbucket_branching_model.test"

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckBitbucketBranchingModelDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBitbucketBranchingModelBranchTypesConfig1(testUser, rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBitbucketBranchingModelExists(resourceName, &branchRestriction),
					resource.TestCheckResourceAttrPair(resourceName, "repository", "bitbucket_repository.test", "name"),
					resource.TestCheckResourceAttr(resourceName, "development.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "development.0.use_mainbranch", "true"),
					resource.TestCheckTypeSetElemNestedAttrs(resourceName, "branch_type.*", map[string]string{
						"kind":   "feature",
						"prefix": "test/",
					}),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// Verify no perpetual diff after apply + import.
				Config:             testAccBitbucketBranchingModelBranchTypesConfig1(testUser, rName),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func TestAccBitbucketBranchingModel_defaultBranchDeletion(t *testing.T) {
	var branchRestriction BranchingModel
	rName := acctest.RandomWithPrefix("tf-test")
	testUser := os.Getenv("BITBUCKET_TEAM")
	resourceName := "bitbucket_branching_model.test"

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders, //nolint:staticcheck // pre-existing repo-wide pattern; ProviderFactories migration is out of scope
		CheckDestroy: testAccCheckBitbucketBranchingModelDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBitbucketBranchingModelDefaultBranchDeletionConfig(testUser, rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckBitbucketBranchingModelExists(resourceName, &branchRestriction),
					resource.TestCheckResourceAttrPair(resourceName, "repository", "bitbucket_repository.test", "name"),
					resource.TestCheckResourceAttr(resourceName, "default_branch_deletion", "true"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// Verify no perpetual diff — this exercises the FlexBool read/write round-trip.
				// A regression here would mean the API returning a string "true" causes Terraform
				// to see a diff on every plan even though nothing has changed.
				Config:             testAccBitbucketBranchingModelDefaultBranchDeletionConfig(testUser, rName),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				// Update to false — exercises the false branch of the FlexBool round-trip against
				// the live API (API may return "false" as a string; must converge to false in state).
				Config: testAccBitbucketBranchingModelFalseDefaultBranchDeletionConfig(testUser, rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "default_branch_deletion", "false"),
				),
			},
			{
				// PlanOnly after setting false — verifies no perpetual diff on the false value.
				Config:             testAccBitbucketBranchingModelFalseDefaultBranchDeletionConfig(testUser, rName),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func testAccBitbucketBranchingModelConfig(testUser, rName string) string {
	return fmt.Sprintf(`
resource "bitbucket_repository" "test" {
  owner = %[1]q
  name  = %[2]q
}
resource "bitbucket_branching_model" "test" {
  owner      = %[1]q
  repository = bitbucket_repository.test.name

  development {
    use_mainbranch = true
  }
}
`, testUser, rName)
}

func testAccBitbucketBranchingModelDefaultBranchDeletionConfig(testUser, rName string) string {
	return fmt.Sprintf(`
resource "bitbucket_repository" "test" {
  owner = %[1]q
  name  = %[2]q
}
resource "bitbucket_branching_model" "test" {
  owner      = %[1]q
  repository = bitbucket_repository.test.name

  default_branch_deletion = true

  development {
    use_mainbranch = true
  }
}
`, testUser, rName)
}

func testAccBitbucketBranchingModelFalseDefaultBranchDeletionConfig(testUser, rName string) string {
	return fmt.Sprintf(`
resource "bitbucket_repository" "test" {
  owner = %[1]q
  name  = %[2]q
}
resource "bitbucket_branching_model" "test" {
  owner      = %[1]q
  repository = bitbucket_repository.test.name

  default_branch_deletion = false

  development {
    use_mainbranch = true
  }
}
`, testUser, rName)
}

func testAccBitbucketBranchingModelProdConfig(testUser, rName string) string {
	return fmt.Sprintf(`
resource "bitbucket_repository" "test" {
  owner = %[1]q
  name  = %[2]q
}
resource "bitbucket_branching_model" "test" {
  owner      = %[1]q
  repository = bitbucket_repository.test.name

  development {
    use_mainbranch = true
  }

  production {
    use_mainbranch = true
	enabled        = true
  }
}
`, testUser, rName)
}

func testAccBitbucketBranchingModelBranchTypesConfig1(testUser, rName string) string {
	return fmt.Sprintf(`
resource "bitbucket_repository" "test" {
  owner = %[1]q
  name  = %[2]q
}
resource "bitbucket_branching_model" "test" {
  owner      = %[1]q
  repository = bitbucket_repository.test.name

  development {
    use_mainbranch = true
  }

  branch_type {
    enabled = true
	kind    = "feature"
	prefix  = "test/"
  }

  branch_type {
    enabled = true
	kind    = "hotfix"
	prefix  = "hotfix/"
  }
 
  branch_type {
    enabled = true
	kind    = "release"
	prefix  = "release/"
  }
 
  branch_type {
    enabled = true
	kind    = "bugfix"
	prefix  = "bugfix/"
  }   
}
`, testUser, rName)
}

func testAccCheckBitbucketBranchingModelDestroy(s *terraform.State) error {
	client := testAccProvider.Meta().(Clients).httpClient
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "bitbucket_branching_model" {
			continue
		}
		response, err := client.Get(fmt.Sprintf("2.0/repositories/%s/%s/branching-model", rs.Primary.Attributes["owner"], rs.Primary.Attributes["repository"]))

		if err == nil {
			return fmt.Errorf("The resource was found should have errored")
		}

		if response.StatusCode != http.StatusNotFound {
			return fmt.Errorf("Branching Model still exists")
		}
	}

	return nil
}

func testAccCheckBitbucketBranchingModelExists(n string, branchRestriction *BranchingModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found %s", n)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("No BranchingModel ID is set")
		}
		return nil
	}
}
