package aws

import (
	"fmt"
	"log"
	"regexp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elastictranscoder"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceAwsElasticTranscoderPipeline() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsElasticTranscoderPipelineCreate,
		Read:   resourceAwsElasticTranscoderPipelineRead,
		Update: resourceAwsElasticTranscoderPipelineUpdate,
		Delete: resourceAwsElasticTranscoderPipelineDelete,

		Schema: map[string]*schema.Schema{
			"arn": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},

			"aws_kms_key_arn": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},

			// ContentConfig also requires ThumbnailConfig
			"content_config": pipelineOutputConfig(),

			"input_bucket": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"name": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
				ValidateFunc: func(v interface{}, k string) (ws []string, errors []error) {
					value := v.(string)
					if !regexp.MustCompile(`^[.0-9A-Za-z-_]+$`).MatchString(value) {
						errors = append(errors, fmt.Errorf(
							"only alphanumeric characters, hyphens, underscores, and periods allowed in %q", k))
					}
					if len(value) > 40 {
						errors = append(errors, fmt.Errorf("%q cannot be longer than 40 characters", k))
					}
					return
				},
			},

			"notifications": &schema.Schema{
				Type:     schema.TypeSet,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"completed": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
						},
						"error": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
						},
						"progressing": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
						},
						"warning": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
						},
					},
				},
			},

			// One of output_bucket or content_config.bucket must be set
			"output_bucket": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},

			"role": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"thumbnail_config": pipelineOutputConfig(),
		},
	}
}

func pipelineOutputConfig() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeSet,
		Optional: true,
		MaxItems: 1,
		Elem: &schema.Resource{
			// elastictranscoder.PipelineOutputConfig
			Schema: map[string]*schema.Schema{
				"bucket": &schema.Schema{
					Type:     schema.TypeString,
					Optional: true,
				},
				"permissions": &schema.Schema{
					Type:     schema.TypeSet,
					Optional: true,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"access": &schema.Schema{
								Type:     schema.TypeList,
								Optional: true,
								Elem:     &schema.Schema{Type: schema.TypeString},
							},
							"grantee": &schema.Schema{
								Type:     schema.TypeString,
								Optional: true,
							},
							"grantee_type": &schema.Schema{
								Type:     schema.TypeString,
								Optional: true,
							},
						},
					},
				},
				"storage_class": &schema.Schema{
					Type:     schema.TypeString,
					Optional: true,
				},
			},
		},
	}
}

func resourceAwsElasticTranscoderPipelineCreate(d *schema.ResourceData, meta interface{}) error {
	elastictranscoderconn := meta.(*AWSClient).elastictranscoderconn

	req := &elastictranscoder.CreatePipelineInput{
		AwsKmsKeyArn:    getStringPtr(d, "aws_kms_key_arn"),
		ContentConfig:   expandETPiplineOutputConfig(d, "content_config"),
		InputBucket:     aws.String(d.Get("input_bucket").(string)),
		Notifications:   expandETNotifications(d),
		OutputBucket:    getStringPtr(d, "output_bucket"),
		Role:            getStringPtr(d, "role"),
		ThumbnailConfig: expandETPiplineOutputConfig(d, "thumbnail_config"),
	}

	if name, ok := d.GetOk("name"); ok {
		req.Name = aws.String(name.(string))
	} else {
		name := resource.PrefixedUniqueId("tf-et-")
		d.Set("name", name)
		req.Name = aws.String(name)
	}

	if (req.OutputBucket == nil && (req.ContentConfig == nil || req.ContentConfig.Bucket == nil)) ||
		(req.OutputBucket != nil && req.ContentConfig != nil && req.ContentConfig.Bucket != nil) {
		return fmt.Errorf("[ERROR] you must specify only one of output_bucket or content_config.bucket")
	}

	log.Printf("[DEBUG] Elastic Transcoder Pipeline create opts: %s", req)
	resp, err := elastictranscoderconn.CreatePipeline(req)
	if err != nil {
		return fmt.Errorf("Error creating Elastic Transcoder Pipeline: %s", err)
	}

	d.SetId(*resp.Pipeline.Id)

	for _, w := range resp.Warnings {
		log.Printf("[WARN] Elastic Transcoder Pipeline %s: %s", w.Code, w.Message)
	}

	return resourceAwsElasticTranscoderPipelineUpdate(d, meta)
}

func expandETNotifications(d *schema.ResourceData) *elastictranscoder.Notifications {
	set, ok := d.GetOk("notifications")
	if !ok {
		return nil
	}

	s := set.(*schema.Set)
	if s == nil || s.Len() == 0 {
		return nil
	}

	m := s.List()[0].(map[string]interface{})

	return &elastictranscoder.Notifications{
		Completed:   getStringPtr(m, "completed"),
		Error:       getStringPtr(m, "error"),
		Progressing: getStringPtr(m, "progressing"),
		Warning:     getStringPtr(m, "warning"),
	}
}

func flattenETNotifications(n *elastictranscoder.Notifications) []map[string]interface{} {
	if n == nil {
		return nil
	}

	allEmpty := func(s ...*string) bool {
		for _, s := range s {
			if s != nil && *s != "" {
				return false
			}
		}
		return true
	}

	// the API always returns a Notifications value, even when all fields are nil
	if allEmpty(n.Completed, n.Error, n.Progressing, n.Warning) {
		return nil
	}

	m := setMap(make(map[string]interface{}))

	m.SetString("completed", n.Completed)
	m.SetString("error", n.Error)
	m.SetString("progressing", n.Progressing)
	m.SetString("warning", n.Warning)
	return m.MapList()
}

func expandETPiplineOutputConfig(d *schema.ResourceData, key string) *elastictranscoder.PipelineOutputConfig {
	set, ok := d.GetOk(key)
	if !ok {
		return nil
	}

	s := set.(*schema.Set)
	if s == nil || s.Len() == 0 {
		return nil
	}

	cc := s.List()[0].(map[string]interface{})

	cfg := &elastictranscoder.PipelineOutputConfig{
		Bucket:       getStringPtr(cc, "bucket"),
		Permissions:  expandETPermList(cc["permissions"].(*schema.Set)),
		StorageClass: getStringPtr(cc, "storage_class"),
	}

	return cfg
}

func flattenETPipelineOutputConfig(cfg *elastictranscoder.PipelineOutputConfig) []map[string]interface{} {
	m := setMap(make(map[string]interface{}))

	m.SetString("bucket", cfg.Bucket)
	m.Set("permissions", flattenETPermList(cfg.Permissions))
	m.SetString("storage_class", cfg.StorageClass)

	return m.MapList()
}

func expandETPermList(permissions *schema.Set) []*elastictranscoder.Permission {
	var perms []*elastictranscoder.Permission

	for _, p := range permissions.List() {
		perm := &elastictranscoder.Permission{
			Access:      getStringPtrList(p.(map[string]interface{}), "access"),
			Grantee:     getStringPtr(p, "grantee"),
			GranteeType: getStringPtr(p, "grantee_type"),
		}
		perms = append(perms, perm)
	}
	return perms
}

func flattenETPermList(perms []*elastictranscoder.Permission) []map[string]interface{} {
	var set []map[string]interface{}

	for _, p := range perms {
		m := setMap(make(map[string]interface{}))
		m.Set("access", flattenStringList(p.Access))
		m.SetString("grantee", p.Grantee)
		m.SetString("grantee_type", p.GranteeType)

		set = append(set, m)
	}
	return set
}

func resourceAwsElasticTranscoderPipelineUpdate(d *schema.ResourceData, meta interface{}) error {
	elastictranscoderconn := meta.(*AWSClient).elastictranscoderconn

	req := &elastictranscoder.UpdatePipelineInput{
		Id: aws.String(d.Id()),
	}

	if d.HasChange("aws_kms_key_arn") {
		req.AwsKmsKeyArn = getStringPtr(d, "aws_kms_key_arn")
	}

	if d.HasChange("content_config") {
		req.ContentConfig = expandETPiplineOutputConfig(d, "content_config")
	}

	if d.HasChange("input_bucket") {
		req.Role = getStringPtr(d, "input_bucket")
	}

	if d.HasChange("name") {
		req.Name = getStringPtr(d, "name")
	}

	if d.HasChange("notifications") {
		req.Notifications = expandETNotifications(d)
	}

	if d.HasChange("role") {
		req.Role = getStringPtr(d, "role")
	}

	if d.HasChange("thumbnail_config") {
		req.ThumbnailConfig = expandETPiplineOutputConfig(d, "thumbnail_config")
	}

	log.Printf("[DEBUG] Updating Elastic Transcoder Pipeline: %#v", req)
	output, err := elastictranscoderconn.UpdatePipeline(req)
	if err != nil {
		return fmt.Errorf("Error updating Elastic Transcoder pipeline: %s", err)
	}

	for _, w := range output.Warnings {
		log.Printf("[WARN] Elastic Transcoder Pipeline %s: %s", w.Code, w.Message)
	}

	return resourceAwsElasticTranscoderPipelineRead(d, meta)
}

func resourceAwsElasticTranscoderPipelineRead(d *schema.ResourceData, meta interface{}) error {
	elastictranscoderconn := meta.(*AWSClient).elastictranscoderconn

	resp, err := elastictranscoderconn.ReadPipeline(&elastictranscoder.ReadPipelineInput{
		Id: aws.String(d.Id()),
	})

	if err != nil {
		if err, ok := err.(awserr.Error); ok && err.Code() == "ResourceNotFoundException" {
			d.SetId("")
			return nil
		}
		return err
	}

	log.Printf("[DEBUG] Elastic Transcoder Pipeline Read response: %#v", resp)

	pipeline := resp.Pipeline

	d.Set("arn", *pipeline.Arn)

	if arn := pipeline.AwsKmsKeyArn; arn != nil {
		d.Set("aws_kms_key_arn", *arn)
	}

	if pipeline.ContentConfig != nil {
		d.Set("content_config", flattenETPipelineOutputConfig(pipeline.ContentConfig))
	}

	d.Set("input_bucket", *pipeline.InputBucket)
	d.Set("name", *pipeline.Name)

	notifications := flattenETNotifications(pipeline.Notifications)
	if notifications != nil {
		if err := d.Set("notifications", notifications); err != nil {
			return fmt.Errorf("error setting notifications: %s", err)
		}
	}

	if pipeline.OutputBucket != nil {
		d.Set("output_bucket", *pipeline.OutputBucket)
	}

	d.Set("role", pipeline.Role)

	if pipeline.ThumbnailConfig != nil {
		d.Set("thumbnail_config", flattenETPipelineOutputConfig(pipeline.ThumbnailConfig))
	}

	return nil
}

func resourceAwsElasticTranscoderPipelineDelete(d *schema.ResourceData, meta interface{}) error {
	elastictranscoderconn := meta.(*AWSClient).elastictranscoderconn

	log.Printf("[DEBUG] Elastic Transcoder Delete Pipeline: %s", d.Id())
	_, err := elastictranscoderconn.DeletePipeline(&elastictranscoder.DeletePipelineInput{
		Id: aws.String(d.Id()),
	})
	if err != nil {
		return fmt.Errorf("error deleting Elastic Transcoder Pipeline: %s", err)
	}
	return nil
}
