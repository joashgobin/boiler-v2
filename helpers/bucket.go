package helpers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
)

type BucketManager struct {
	client     *s3.Client
	presigner  *s3.PresignClient
	bucketName string
	publicURL  string
	config     *aws.Config
}

type BucketManagerInterface interface {
	Ping()
	GetObjects() []Object
	GetBuckets() []string
}

var _ BucketManagerInterface = (*BucketManager)(nil)

func (bm *BucketManager) Ping() {}

func (bm *BucketManager) GetBuckets() []string {
	var bucketNames []string
	resp, err := bm.client.ListBuckets(context.Background(), &s3.ListBucketsInput{})
	if err != nil {
		log.Error(err)
		return []string{}
	}
	for _, bucket := range resp.Buckets {
		bucketNames = append(bucketNames, *bucket.Name)
	}
	return bucketNames
}

type Object struct {
	Key          string    `json:"Key"`
	LastModified time.Time `json:"LastModified"`
	Size         int       `json:"Size"`
	URL          string
}

func (bm *BucketManager) GetObjects() []Object {

	listObjectsOutput, err := bm.client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket: &bm.bucketName,
	})
	if err != nil {
		log.Fatal(err)
	}

	contents := listObjectsOutput.Contents
	objects := make([]Object, 0, len(contents))
	for _, object := range contents {
		var newObject Object
		newObject.Key = *object.Key
		newObject.LastModified = *object.LastModified
		newObject.Size = int(*object.Size)
		newObject.URL = bm.publicURL + "/" + *object.Key
		objects = append(objects, newObject)
	}

	//  {
	//    "ChecksumAlgorithm": null,
	//    "ETag": "\"eb2b891dc67b81755d2b726d9110af16\"",
	//    "Key": "ferriswasm.png",
	//    "LastModified": "2022-05-18T17:20:21.67Z",
	//    "Owner": null,
	//    "Size": 87671,
	//    "StorageClass": "STANDARD"
	//  }
	return objects

}

func NewBucketManager(bucketName string) *BucketManager {
	// Provide your Cloudflare account ID
	var accountId = helpers.GetEnv("R2_ACCOUNT_ID")
	// Retrieve your S3 API credentials for your R2 bucket via API tokens
	// (see: https://developers.cloudflare.com/r2/api/tokens)
	var accessKeyId = helpers.GetEnv("R2_ACCESS_KEY_ID")
	var accessKeySecret = helpers.GetEnv("R2_SECRET_ACCESS_KEY")

	var publicURL = helpers.GetEnv("R2_PUBLIC_URL")

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyId, accessKeySecret, "")),
		config.WithRegion("auto"), // Required by SDK but not used by R2
	)
	if err != nil {
		log.Fatal(err)
	}

	var endpointBuilder strings.Builder
	endpointBuilder.WriteString("https://")
	endpointBuilder.WriteString(accountId)
	endpointBuilder.WriteString(".r2.cloudflarestorage.com")

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpointBuilder.String())
	})

	presigner := s3.NewPresignClient(client)

	return &BucketManager{
		client:     client,
		bucketName: bucketName,
		publicURL:  publicURL,
		config:     &cfg,
		presigner:  presigner,
	}
}

func (bm *BucketManager) getPutURL(key string) (string, error) {
	res, err := bm.presigner.PresignPutObject(context.Background(), &s3.PutObjectInput{
		Bucket: &bm.bucketName,
		Key:    aws.String(key),
	}, s3.WithPresignExpires(15*time.Minute))
	if err != nil {
		return "", fmt.Errorf("upload object error: %v", err)
	}
	return res.URL, nil
}

func (bm *BucketManager) Upload(c fiber.Ctx, key string) error {
	url, err := bm.getPutURL(key)
	if err != nil {
		return c.SendString(fmt.Sprintf("Error uploading: %v", err))
	}
	file, err := c.FormFile("file")
	if err != nil {
		return c.SendString(fmt.Sprintf("Error uploading: %v", err))
	}
	fileStream, err := file.Open()
	if err != nil {
		return c.SendString(fmt.Sprintf("Error uploading: %v", err))
	}
	defer fileStream.Close()

	if err != nil {
		return c.SendString(fmt.Sprintf("Error uploading: %v", err))
	}
	err = bm.uploadPayload(url, fileStream, file.Size, file.Header.Get("Content-Type"))
	if err != nil {
		return c.SendString(fmt.Sprintf("Error uploading: %v", err))
	}
	return nil
}

func (bm *BucketManager) downloadObject(key string) (string, error) {
	result, err := bm.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(bm.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", err
	}
	defer result.Body.Close()

	// Read stream into a string
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, result.Body)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (bm *BucketManager) uploadPayload(url string, payload io.Reader, payloadSize int64, contentType string) error {
	req, err := http.NewRequest(http.MethodPut, url, payload)
	if err != nil {
		return err
	}

	req.ContentLength = payloadSize

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code from S3: %s", resp.Status)
	}

	return nil
}
