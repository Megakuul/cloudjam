package aws

// goformation models optional CloudFormation properties as pointers, so setting
// one inline needs an addressable value. These helpers keep resource literals
// readable:
//
//	&s3.Bucket{BucketName: aws.String("cloudjam-demo")}

// String returns a pointer to v.
func String(v string) *string { return &v }

// Bool returns a pointer to v.
func Bool(v bool) *bool { return &v }

// Int returns a pointer to v.
func Int(v int) *int { return &v }

// Float64 returns a pointer to v.
func Float64(v float64) *float64 { return &v }

// Value dereferences p, returning the zero value of T when p is nil. It reads
// well in checks against live state, where every optional property is a pointer
// that may legitimately be unset:
//
//	if aws.Value(bucket.BucketName) == "cloudjam-demo" { ... }
func Value[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// True reports whether p is set and true — the common shape of a boolean
// guardrail check:
//
//	aws.True(cfg.BlockPublicAcls) && aws.True(cfg.BlockPublicPolicy)
func True(p *bool) bool { return p != nil && *p }
