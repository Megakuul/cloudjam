// just for testing cloud apis and projects
package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
)

func main() {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-east-1"))
	if err != nil {
		panic(err)
	}

	client := cloudcontrol.NewFromConfig(cfg)
	client.CreateResource(context.TODO(), &cloudcontrol.CreateResourceInput{
		TypeName:     aws.String("AWS::S3::Bucket"),
		DesiredState: aws.String(""),
	})
}
