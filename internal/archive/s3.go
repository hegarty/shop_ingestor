// Package archive writes raw Shopify payloads to S3, durably and cheaply,
// so the platform can replay history independent of Shopify's own API
// retention and independent of Redpanda's (short) retention. See
// shop_docs/docs/disaster-recovery.md.
package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/hegarty/shop_platform/event"
)

// Writer archives RawEnvelopes to a single S3 bucket, partitioned by
// source/tenant/event type/date — see shop_docs/docs/architecture.md.
type Writer struct {
	client *s3.Client
	bucket string
}

func NewWriter(client *s3.Client, bucket string) *Writer {
	return &Writer{client: client, bucket: bucket}
}

// Put archives one RawEnvelope. The object key is fully determined by the
// envelope's own fields, so archiving the same event twice (a duplicate
// webhook delivery that reached here before idempotency checks ran)
// overwrites the same key rather than accumulating duplicates.
func (w *Writer) Put(ctx context.Context, env event.RawEnvelope) error {
	key := ObjectKey(env)

	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("archive: marshal envelope %s: %w", env.EventID, err)
	}

	_, err = w.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(w.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("archive: put %s: %w", key, err)
	}
	return nil
}

// ObjectKey computes the S3 key for a RawEnvelope. Exported so tests (and
// any future reconciliation-side lookup) can predict where an event landed
// without needing a real S3 client.
func ObjectKey(env event.RawEnvelope) string {
	sanitizedEventType := strings.ReplaceAll(env.EventType, "/", "_")
	t := env.ReceivedAt.UTC()
	return fmt.Sprintf(
		"source=%s/tenant=%s/event_type=%s/year=%04d/month=%02d/day=%02d/%s.json",
		env.Source, env.TenantID, sanitizedEventType,
		t.Year(), t.Month(), t.Day(),
		env.EventID,
	)
}
