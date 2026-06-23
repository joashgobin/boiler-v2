package helpers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
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
	client      *s3.Client
	presigner   *s3.PresignClient
	bucketName  string
	publicURL   string
	config      *aws.Config
	bank        BankInterface
	cacheExpiry time.Duration
}

type BucketManagerInterface interface {
	Ping()
	GetObjects(folderPath ...string) []Object
	GetObjectsCached(folderPath ...string) []Object
	GetBuckets() []string
	ClearCache()
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
	Name         string
	IsFolder     bool
}

func (bm *BucketManager) GetObjectsCached(folderPath ...string) []Object {
	prefix := ""
	if len(folderPath) > 0 {
		prefix = folderPath[0]
	}

	cacheKey := "r2-objects-" + prefix
	cachedObjects := BytesToSlice[Object](bm.bank.GetBytes(cacheKey))
	if len(cachedObjects) == 0 {
		cachedObjects = bm.GetObjects(folderPath...)
		bm.bank.SetBytes(cacheKey, SliceToBytes[Object](cachedObjects), bm.cacheExpiry)
	}
	return cachedObjects
}

func (bm *BucketManager) ClearCache() {
	// TODO: make record of keys for various prefixes and delete them in bulk
	cacheKey := "r2-objects-"
	bm.bank.Delete(cacheKey)
}

func (bm *BucketManager) GetObjects(folderPath ...string) []Object {
	delimiter := "/"
	input := &s3.ListObjectsV2Input{
		Bucket:    &bm.bucketName,
		Delimiter: &delimiter,
	}

	if len(folderPath) > 0 {
		if folderPath[0] != "" {
			prefix := folderPath[0]
			// append slash if not present
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}

			// use new input to account for prefix
			input = &s3.ListObjectsV2Input{
				Bucket:    &bm.bucketName,
				Prefix:    &prefix,
				Delimiter: &delimiter,
			}
		}
	}

	listObjectsOutput, err := bm.client.ListObjectsV2(context.Background(), input)
	if err != nil {
		log.Errorf("get bucket objects error: %v", err)
		return []Object{}
	}

	folders := listObjectsOutput.CommonPrefixes
	contents := listObjectsOutput.Contents
	objects := make([]Object, 0, len(contents)+len(folders))

	// include folders
	for _, object := range folders {
		var newObject Object
		newObject.Key = *object.Prefix
		newObject.Name = filepath.Base(*object.Prefix)
		newObject.IsFolder = true
		objects = append(objects, newObject)
	}

	// include regular objects
	for _, object := range contents {
		var newObject Object
		newObject.Key = *object.Key
		if strings.HasSuffix(newObject.Key, "/") {
			continue
		}
		newObject.Name = filepath.Base(*object.Key)
		newObject.IsFolder = false
		// set other fields for non-folder objects
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

func NewBucketManager(bucketName string, bank BankInterface) *BucketManager {
	// Provide your Cloudflare account ID
	var accountId = GetEnv("R2_ACCOUNT_ID")
	// Retrieve your S3 API credentials for your R2 bucket via API tokens
	// (see: https://developers.cloudflare.com/r2/api/tokens)
	var accessKeyId = GetEnv("R2_ACCESS_KEY_ID")
	var accessKeySecret = GetEnv("R2_SECRET_ACCESS_KEY")

	var publicURL = GetEnv("R2_PUBLIC_URL")

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

	bm := &BucketManager{
		client:      client,
		bucketName:  bucketName,
		publicURL:   publicURL,
		config:      &cfg,
		presigner:   presigner,
		bank:        bank,
		cacheExpiry: time.Minute * 15,
	}
	bm.ClearCache()
	return bm
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
	// get presigned PUT url
	url, err := bm.getPutURL(key)
	if err != nil {
		return c.SendString(fmt.Sprintf("Error uploading: %v", err))
	}

	// get file submitted via form
	file, err := c.FormFile("file")
	if err != nil {
		return c.SendString(fmt.Sprintf("Error uploading: %v", err))
	}

	// get file stream
	fileStream, err := file.Open()
	if err != nil {
		return c.SendString(fmt.Sprintf("Error uploading: %v", err))
	}
	defer fileStream.Close()

	// upload payload to bucket
	err = bm.uploadPayload(url, fileStream, file.Size, file.Header.Get("Content-Type"))
	if err != nil {
		return c.SendString(fmt.Sprintf("Error uploading: %v", err))
	}

	// clear object cache
	bm.ClearCache()
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

	// read stream into a string
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, result.Body)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (bm *BucketManager) uploadPayload(url string, payload io.Reader, payloadSize int64, contentType string) error {
	// create new request
	req, err := http.NewRequest(http.MethodPut, url, payload)
	if err != nil {
		return err
	}

	// set request content length
	req.ContentLength = payloadSize

	// create http client
	client := TimedHTTPClient()
	// get http response
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// check response code
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code from S3: %s", resp.Status)
	}
	return nil
}
