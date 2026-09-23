package bitbucket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/DrFaust92/bitbucket-go-client"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceProject() *schema.Resource {
	return &schema.Resource{
		CreateWithoutTimeout: resourceProjectCreate,
		UpdateWithoutTimeout: resourceProjectUpdate,
		ReadWithoutTimeout:   resourceProjectRead,
		DeleteWithoutTimeout: resourceProjectDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"key": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"is_private": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"owner": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"name": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"has_publicly_visible_repos": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"uuid": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"link": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"avatar": {
							Type:     schema.TypeList,
							Optional: true,
							MaxItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"href": {
										Type:     schema.TypeString,
										Optional: true,
										DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
											return strings.HasPrefix(old, "https://bitbucket.org/account/user")
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// projectWriteBody is used for POST and PUT requests instead of bitbucket.Project.
// bitbucket-go-client v0.11.0 incorrectly serializes created_on, updated_on, and
// type into the request body; Bitbucket rejects them as "extra keys not allowed".
// See: https://github.com/DrFaust92/bitbucket-go-client/issues/41
// Remove this struct and revert to genClient once the upstream client is fixed.
type projectWriteBody struct {
	Description string                  `json:"description,omitempty"`
	IsPrivate   bool                    `json:"is_private"` // no omitempty: false must be sent explicitly on PUT to avoid Bitbucket preserving the existing value
	Key         string                  `json:"key,omitempty"`
	Links       *bitbucket.ProjectLinks `json:"links,omitempty"`
	Name        string                  `json:"name,omitempty"`
}

func newProjectWriteBody(d *schema.ResourceData) *projectWriteBody {
	p := &projectWriteBody{
		Name:        d.Get("name").(string),
		IsPrivate:   d.Get("is_private").(bool),
		Description: d.Get("description").(string),
		Key:         d.Get("key").(string),
	}
	if v, ok := d.GetOk("link"); d.IsNewResource() && ok && len(v.([]interface{})) > 0 && v.([]interface{}) != nil {
		p.Links = expandProjectLinks(v.([]interface{}))
	}
	return p
}

func resourceProjectUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(Clients).httpClient
	owner := d.Get("owner").(string)
	key := d.Get("key").(string)

	body := newProjectWriteBody(d)
	body.Links = nil // preserve existing behaviour: links not updated via PUT body

	log.Printf("[DEBUG] Project Update Body: %#v", body)

	payload, err := json.Marshal(body)
	if err != nil {
		return diag.FromErr(err)
	}

	_, err = client.Put(
		fmt.Sprintf("2.0/workspaces/%s/projects/%s", owner, key),
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceProjectRead(ctx, d, m)
}

func resourceProjectCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(Clients).httpClient
	owner := d.Get("owner").(string)

	log.Printf("[DEBUG] Project Create Body: %#v", newProjectWriteBody(d))

	payload, err := json.Marshal(newProjectWriteBody(d))
	if err != nil {
		return diag.FromErr(err)
	}

	// client.Post returns a typed error for any non-2xx response (see Client.Do),
	// so the Decode below only runs on a successful 2xx response.
	res, err := client.Post(
		fmt.Sprintf("2.0/workspaces/%s/projects", owner),
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return diag.FromErr(err)
	}

	var created bitbucket.Project
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(fmt.Sprintf("%s/%s", owner, created.Key))

	return resourceProjectRead(ctx, d, m)
}

func resourceProjectRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	id := d.Id()
	if id != "" {
		idparts := strings.Split(id, "/")
		if len(idparts) == 2 {
			d.Set("owner", idparts[0])
			d.Set("key", idparts[1])
		} else {
			return diag.Errorf("incorrect ID format, should match `owner/key`")
		}
	}

	var projectKey string
	projectKey = d.Get("key").(string)
	if projectKey == "" {
		projectKey = d.Get("key").(string)
	}

	c := m.(Clients).genClient
	projectApi := c.ApiClient.ProjectsApi

	projRes, res, err := projectApi.WorkspacesWorkspaceProjectsProjectKeyGet(c.AuthContext, projectKey, d.Get("owner").(string))

	if res != nil && res.StatusCode == http.StatusNotFound {
		log.Printf("[WARN] Project (%s) not found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err := handleClientError(res, err); err != nil {
		return diag.FromErr(err)
	}

	d.Set("key", projRes.Key)
	d.Set("is_private", projRes.IsPrivate)
	d.Set("name", projRes.Name)
	d.Set("description", projRes.Description)
	d.Set("has_publicly_visible_repos", projRes.HasPubliclyVisibleRepos)
	d.Set("uuid", projRes.Uuid)
	d.Set("link", flattenProjectLinks(projRes.Links))

	return nil
}

func resourceProjectDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {

	var projectKey string
	projectKey = d.Get("key").(string)
	if projectKey == "" {
		projectKey = d.Get("key").(string)
	}

	c := m.(Clients).genClient
	projectApi := c.ApiClient.ProjectsApi

	res, err := projectApi.WorkspacesWorkspaceProjectsProjectKeyDelete(c.AuthContext, projectKey, d.Get("owner").(string))
	if err := handleClientError(res, err); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func expandProjectLinks(l []interface{}) *bitbucket.ProjectLinks {
	if len(l) == 0 || l[0] == nil {
		return nil
	}

	tfMap, ok := l[0].(map[string]interface{})

	if !ok {
		return nil
	}

	rp := &bitbucket.ProjectLinks{}

	if v, ok := tfMap["avatar"].([]interface{}); ok && len(v) > 0 {
		rp.Avatar = expandLink(v)
	}

	return rp
}

func flattenProjectLinks(rp *bitbucket.ProjectLinks) []interface{} {
	if rp == nil {
		return []interface{}{}
	}

	m := map[string]interface{}{
		"avatar": flattenLink(rp.Avatar),
	}

	return []interface{}{m}
}
