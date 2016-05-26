package aws

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elastictranscoder"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
)

func TestAccAWSElasticTranscoderPreset(t *testing.T) {
	preset := &elastictranscoder.Preset{}

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckElasticTranscoderPresetDestroy,
		Steps: []resource.TestStep{
			resource.TestStep{
				Config: awsElasticTranscoderPresetConfig,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAWSElasticTranscoderPresetExists("aws_elastictranscoder_preset.bar", preset),
				),
			},
		},
	})
}

func testAccCheckAWSElasticTranscoderPresetExists(n string, res *elastictranscoder.Preset) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No Preset ID is set")
		}

		conn := testAccProvider.Meta().(*AWSClient).elastictranscoderconn

		out, err := conn.ReadPreset(&elastictranscoder.ReadPresetInput{
			Id: aws.String(rs.Primary.ID),
		})

		if err != nil {
			return err
		}

		res = out.Preset

		return nil
	}
}

func testAccCheckElasticTranscoderPresetDestroy(s *terraform.State) error {
	conn := testAccProvider.Meta().(*AWSClient).elastictranscoderconn

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "aws_elastictranscoder_preset" {
			continue
		}

		out, err := conn.ReadPreset(&elastictranscoder.ReadPresetInput{
			Id: aws.String(rs.Primary.ID),
		})

		if err == nil {
			if out.Preset != nil && *out.Preset.Id == rs.Primary.ID {
				return fmt.Errorf("Elastic Transcoder Preset still exists")
			}
		}

		awsErr, ok := err.(awserr.Error)
		if !ok {
			return err
		}

		if awsErr.Code() != "ResourceNotFoundException" {
			return fmt.Errorf("unexpected error: %s", awsErr)
		}

	}
	return nil
}

const awsElasticTranscoderPresetConfig = `
resource "aws_elastictranscoder_preset" "bar" {
  container   = "mp4"
  description = "aws_elastictranscoder_preset_tf_test_"
  name        = "aws_elastictranscoder_preset_tf_test_"
  audio = {
    audio_packing_mode = "SingleTrack"
    bit_rate = 320
	channels = 2
	codec = "mp3"
	sample_rate = 44100
  }
}`
