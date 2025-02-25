# WhatAmIListeningTo API

A Go-based backend for a music sharing application that lets users share their current Spotify tracks in real-time.

## Features

- **Spotify Integration**: Connect with your Spotify account to automatically share what you're listening to
- **Real-time Updates**: WebSocket support for live updates when songs change
- **User Profiles**: Customizable profile pages to share your music with friends
- **Track History**: Keep a record of previously played tracks
- **Visitor Statistics**: See how many people are viewing your profile

## Tech Stack

- **Go**: Backend language
- **Gin**: Web framework
- **PostgreSQL**: Persistent data storage
- **Redis**: Caching and real-time pub/sub
- **WebSockets**: Real-time client updates

## Installation

### Prerequisites

- Go 1.19+
- PostgreSQL
- Redis
- Spotify Developer Account

### Setup

1. Clone the repository
```bash
git clone https://github.com/brandonhuynh1/whatamilisteningto-api.git
cd whatamilisteningto-api
```

2. Install dependencies
```bash
go mod tidy
```

3. Create a ```.env``` file based on ```.env.example```
```bash
cp .env.example .env
```

4. Update the ```.env``` with your credentials

5. Set up the database

## Running the Application
```bash
go run ./cmd/server
```

The server will start on http://localhost:8080 (or whatever port you configured).

## API Endpoints

### Authentication
* `GET /auth/spotify`: Initiate Spotify OAuth flow
* `GET /auth/spotify/callback`: Spotify OAuth callback
* `GET /auth/logout`: Log out user
* `GET /auth/status`: Check authentication status

### Profiles
* `GET /profile/:profileURL`: View a user's public profile
* `GET /api/profile`: Get authenticated user's profile
* `PUT /api/profile`: Update authenticated user's profile
* `PUT /api/profile/settings`: Update sharing settings

### Tracks
* `GET /ws/tracks/:profileURL`: WebSocket endpoint for real-time track updates
* `GET /api/tracks/current`: Get currently playing track
* `GET /api/tracks/history`: Get track history
* `POST /api/tracks/refresh`: Manually refresh current track

