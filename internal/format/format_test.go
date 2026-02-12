package format

import (
	"testing"
	"time"
)

func TestSize(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{
			name:  "bytes",
			bytes: 500,
			want:  "500 B",
		},
		{
			name:  "kilobytes",
			bytes: 1024 * 5,
			want:  "5.00 KB",
		},
		{
			name:  "megabytes",
			bytes: 1024 * 1024 * 10,
			want:  "10.00 MB",
		},
		{
			name:  "gigabytes",
			bytes: 1024 * 1024 * 1024 * 2,
			want:  "2.00 GB",
		},
		{
			name:  "fractional gigabytes",
			bytes: int64(1.5 * 1024 * 1024 * 1024),
			want:  "1.50 GB",
		},
		{
			name:  "zero bytes",
			bytes: 0,
			want:  "0 B",
		},
		{
			name:  "large image size (typical bootc)",
			bytes: 1073741824, // 1 GB
			want:  "1.00 GB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Size(tt.bytes)
			if got != tt.want {
				t.Errorf("Size(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestTimeAgo(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		unixTime int64
		want     string
	}{
		{
			name:     "zero timestamp",
			unixTime: 0,
			want:     "N/A",
		},
		{
			name:     "less than a minute ago",
			unixTime: now.Add(-30 * time.Second).Unix(),
			want:     "Less than a minute ago",
		},
		{
			name:     "1 minute ago",
			unixTime: now.Add(-1 * time.Minute).Unix(),
			want:     "1 minute ago",
		},
		{
			name:     "5 minutes ago",
			unixTime: now.Add(-5 * time.Minute).Unix(),
			want:     "5 minutes ago",
		},
		{
			name:     "1 hour ago",
			unixTime: now.Add(-1 * time.Hour).Unix(),
			want:     "1 hour ago",
		},
		{
			name:     "3 hours ago",
			unixTime: now.Add(-3 * time.Hour).Unix(),
			want:     "3 hours ago",
		},
		{
			name:     "1 day ago",
			unixTime: now.Add(-24 * time.Hour).Unix(),
			want:     "1 day ago",
		},
		{
			name:     "3 days ago",
			unixTime: now.Add(-3 * 24 * time.Hour).Unix(),
			want:     "3 days ago",
		},
		{
			name:     "1 week ago",
			unixTime: now.Add(-7 * 24 * time.Hour).Unix(),
			want:     "1 week ago",
		},
		{
			name:     "2 weeks ago",
			unixTime: now.Add(-14 * 24 * time.Hour).Unix(),
			want:     "2 weeks ago",
		},
		{
			name:     "1 month ago",
			unixTime: now.Add(-30 * 24 * time.Hour).Unix(),
			want:     "1 month ago",
		},
		{
			name:     "3 months ago",
			unixTime: now.Add(-90 * 24 * time.Hour).Unix(),
			want:     "3 months ago",
		},
		{
			name:     "1 year ago",
			unixTime: now.Add(-365 * 24 * time.Hour).Unix(),
			want:     "1 year ago",
		},
		{
			name:     "2 years ago",
			unixTime: now.Add(-730 * 24 * time.Hour).Unix(),
			want:     "2 years ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TimeAgo(tt.unixTime)
			if got != tt.want {
				t.Errorf("TimeAgo(%d) = %q, want %q", tt.unixTime, got, tt.want)
			}
		})
	}
}
