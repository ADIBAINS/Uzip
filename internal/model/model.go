package model

import "time"

type Principal struct {
	UserID        string
	TenantID      string
	Role          string
	Email         string
	EmailVerified bool
}

func (p Principal) VerifiedEmail() string {
	if p.EmailVerified {
		return p.Email
	}
	return ""
}

type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

type Link struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	Alias       string     `json:"alias"`
	Destination string     `json:"destination"`
	Redirect    int        `json:"redirect_type"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (l Link) Active(now time.Time) bool {
	return l.ArchivedAt == nil && (l.ExpiresAt == nil || l.ExpiresAt.After(now))
}

type CreateLink struct {
	Alias       string     `json:"alias"`
	Destination string     `json:"destination"`
	Redirect    int        `json:"redirect_type"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

type Notification struct {
	ID        string     `json:"id"`
	LinkID    *string    `json:"link_id,omitempty"`
	Alias     string     `json:"alias"`
	Kind      string     `json:"kind"`
	Milestone int        `json:"milestone,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type ClickEvent struct {
	SchemaVersion int               `json:"schema_version"`
	EventID       string            `json:"event_id"`
	OccurredAt    time.Time         `json:"occurred_at"`
	TenantID      string            `json:"tenant_id"`
	LinkID        string            `json:"link_id"`
	Alias         string            `json:"alias"`
	VisitorID     string            `json:"visitor_id"`
	IPAddress     string            `json:"ip_address,omitempty"`
	UserAgent     string            `json:"user_agent,omitempty"`
	Referrer      string            `json:"referrer,omitempty"`
	Country       string            `json:"country,omitempty"`
	Device        string            `json:"device,omitempty"`
	Browser       string            `json:"browser,omitempty"`
	OS            string            `json:"os,omitempty"`
	IsBot         bool              `json:"is_bot,omitempty"`
	TraceParent   string            `json:"trace_parent,omitempty"`
	Properties    map[string]string `json:"properties,omitempty"`
}

type AnalyticsSummary struct {
	TotalClicks    uint64 `json:"total_clicks"`
	UniqueVisitors uint64 `json:"unique_visitors"`
}

type TimePoint struct {
	Timestamp time.Time `json:"timestamp"`
	Clicks    uint64    `json:"clicks"`
	Unique    uint64    `json:"unique_visitors"`
}

type Breakdown struct {
	Name   string `json:"name"`
	Clicks uint64 `json:"clicks"`
}

type APIKey struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}
