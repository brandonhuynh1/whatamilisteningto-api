package models

import (
	"time"
)

// User represents a registered user in the system
type User struct {
	ID                  string    `json:"id" db:"id"`
	SpotifyID           string    `json:"spotify_id" db:"spotify_id"`
	Email               string    `json:"email" db:"email"`
	DisplayName         string    `json:"display_name" db:"display_name"`
	ProfileURL          string    `json:"profile_url" db:"profile_url"`
	SpotifyAccessToken  string    `json:"-" db:"spotify_access_token"`
	SpotifyRefreshToken string    `json:"-" db:"spotify_refresh_token"`
	TokenExpiresAt      time.Time `json:"-" db:"token_expires_at"`
	IsActive            bool      `json:"is_active" db:"is_active"`
	IsSharingEnabled    bool      `json:"is_sharing_enabled" db:"is_sharing_enabled"`
	CreatedAt           time.Time `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time `json:"updated_at" db:"updated_at"`
}

// Profile represents user profile customization
type Profile struct {
	ID              string    `json:"id" db:"id"`
	UserID          string    `json:"user_id" db:"user_id"`
	Theme           string    `json:"theme" db:"theme"`
	BackgroundColor string    `json:"background_color" db:"background_color"`
	TextColor       string    `json:"text_color" db:"text_color"`
	CustomMessage   string    `json:"custom_message" db:"custom_message"`
	ShowStats       bool      `json:"show_stats" db:"show_stats"`
	ShowHistory     bool      `json:"show_history" db:"show_history"`
	AnimationStyle  string    `json:"animation_style" db:"animation_style"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

// Track represents a song that a user has played or is playing
type Track struct {
	ID                 string    `json:"id" db:"id"`
	UserID             string    `json:"user_id" db:"user_id"`
	SpotifyTrackID     string    `json:"spotify_track_id" db:"spotify_track_id"`
	Name               string    `json:"name" db:"name"`
	Artist             string    `json:"artist" db:"artist"`
	Album              string    `json:"album" db:"album"`
	AlbumArtURL        string    `json:"album_art_url" db:"album_art_url"`
	TrackURL           string    `json:"track_url" db:"track_url"`
	DurationMs         int       `json:"duration_ms" db:"duration_ms"`
	IsCurrentlyPlaying bool      `json:"is_currently_playing" db:"is_currently_playing"`
	PlayedAt           time.Time `json:"played_at" db:"played_at"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
}

// ProfileVisit tracks profile visits by anonymous users
type ProfileVisit struct {
	ID            string     `json:"id" db:"id"`
	UserID        string     `json:"user_id" db:"user_id"`
	VisitorIP     string     `json:"-" db:"visitor_ip"`
	VisitorUserID *string    `json:"visitor_user_id,omitempty" db:"visitor_user_id"`
	UserAgent     string     `json:"-" db:"user_agent"`
	ReferrerURL   string     `json:"referrer_url" db:"referrer_url"`
	StartedAt     time.Time  `json:"started_at" db:"started_at"`
	EndedAt       *time.Time `json:"ended_at,omitempty" db:"ended_at"`
}

// SpotifyCurrentlyPlaying represents the currently playing track from Spotify API
type SpotifyCurrentlyPlaying struct {
	IsPlaying   bool   `json:"is_playing"`
	TrackID     string `json:"track_id"`
	TrackName   string `json:"track_name"`
	ArtistName  string `json:"artist_name"`
	AlbumName   string `json:"album_name"`
	AlbumArtURL string `json:"album_art_url"`
	TrackURL    string `json:"track_url"`
	DurationMs  int    `json:"duration_ms"`
	ProgressMs  int    `json:"progress_ms"`
}

// ProfileResponse represents the data sent to profile visitors
type ProfileResponse struct {
	User         UserPublic `json:"user"`
	Profile      Profile    `json:"profile"`
	CurrentTrack *Track     `json:"current_track,omitempty"`
	RecentTracks []Track    `json:"recent_tracks,omitempty"`
	ViewerCount  int        `json:"viewer_count"`
}

// UserPublic represents the public information about a user
type UserPublic struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	ProfileURL  string `json:"profile_url"`
}
