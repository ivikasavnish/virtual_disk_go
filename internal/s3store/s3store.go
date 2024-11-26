package s3store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Store represents an S3-compatible storage backend
type S3Store struct {
	client     *s3.Client
	bucketName string
	prefix     string
}

// NewS3Store creates a new S3Store instance
func NewS3Store(endpoint, region, bucketName, prefix string) (*S3Store, error) {
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, reg string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:           endpoint,
			SigningRegion: region,
		}, nil
	})

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithEndpointResolverWithOptions(customResolver),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	return &S3Store{
		client:     client,
		bucketName: bucketName,
		prefix:     prefix,
	}, nil
}

// WriteFile writes data to an S3 object
func (s *S3Store) WriteFile(path string, data []byte) error {
	key := s.getObjectKey(path)
	_, err := s.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("failed to write to S3: %w", err)
	}
	return nil
}

// ReadFile reads data from an S3 object
func (s *S3Store) ReadFile(path string) ([]byte, error) {
	key := s.getObjectKey(path)
	output, err := s.client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read from S3: %w", err)
	}
	defer output.Body.Close()

	return io.ReadAll(output.Body)
}

// DeleteFile deletes an S3 object
func (s *S3Store) DeleteFile(path string) error {
	key := s.getObjectKey(path)
	_, err := s.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete from S3: %w", err)
	}
	return nil
}

// ListFiles lists objects in S3 with the given prefix
func (s *S3Store) ListFiles(prefix string) ([]string, error) {
	fullPrefix := s.getObjectKey(prefix)
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucketName),
		Prefix: aws.String(fullPrefix),
	})

	var files []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.TODO())
		if err != nil {
			return nil, fmt.Errorf("failed to list S3 objects: %w", err)
		}

		for _, obj := range page.Contents {
			key := *obj.Key
			if strings.HasPrefix(key, s.prefix) {
				relPath := strings.TrimPrefix(key, s.prefix)
				relPath = strings.TrimPrefix(relPath, "/")
				files = append(files, relPath)
			}
		}
	}

	return files, nil
}

func (s *S3Store) getObjectKey(path string) string {
	if s.prefix == "" {
		return path
	}
	return fmt.Sprintf("%s/%s", strings.TrimSuffix(s.prefix, "/"), strings.TrimPrefix(path, "/"))
}
