package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// TODO: implement the upload here
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid upload file", err)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid upload file", err)
		return
	}
	mediaType := header.Header.Get("Content-Type")
	mediaType = http.DetectContentType(data)

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't find video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You are not authorized to upload this video", err)
		return
	}

	videoThumbnails[videoID] = thumbnail{
		data:      data,
		mediaType: mediaType,
	}

	// Update the video metadata so that it has a new thumbnail URL,
	// then update the record in the database by using the cfg.db.UpdateVideo function. The thumbnail URL should have this format:
	// http://localhost:<port>/api/thumbnails/{videoID}
	// This will all work because the /api/thumbnails/{videoID} endpoint serves thumbnails from that global `videoThumbnails` map.
	thumbURL := fmt.Sprintf("http://%s/api/thumbnails/%s", r.Host, videoID.String())
	video.ThumbnailURL = &thumbURL

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
