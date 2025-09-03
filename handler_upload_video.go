package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	// set an upload limit of 1 GB (1 << 30 bytes) using http.MaxBytesReader.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

	// Extract the videoID from the URL path parameters and parse it as a UUID
	videoIdStr := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIdStr)
	if err != nil {
		respondWithError(w, 500, "couldn't parse video id", err)
		return
	}

	// Authenticate the user to get a userID
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}
	userId, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	// todo: Get the video metadata from the database, if the user is not the video owner, return a http.StatusUnauthorized response
	vid, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Video not found", err)
		return
	}
	if vid.UserID != userId {
		respondWithError(w, http.StatusUnauthorized, "You are not authorized to upload this video", nil)
		return
	}

	// Parse the uploaded video file from the form data
	// - Use (http.Request).FormFile with the key "video" to get a multipart.File in memory
	// - Remember to defer closing the file with (os.File).Close - we don't want any memory leaks
	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	// Validate the uploaded file to ensure it's an MP4 video
	// - Use mime.ParseMediaType and "video/mp4" as the MIME type
	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Unsupported Media Type", nil)
		return
	}

	// Save the uploaded file to a temporary file on disk.
	// - Use os.CreateTemp to create a temporary file. I passed in an empty string for the directory to use the system default, and the name "tubely-upload.mp4" (but you can use whatever you want)
	// - defer remove the temp file with os.Remove
	// - defer close the temp file (defer is LIFO, so it will close before the remove)
	// - io.Copy the contents over from the wire to the temp file
	dst, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create temp file", err)
		return
	}
	defer func() {
		dst.Close()
		_ = os.Remove(dst.Name())
	}()
	_, err = io.Copy(dst, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to write to temp file", err)
		return
	}

	// Reset the tempFile's file pointer to the beginning with .Seek(0, io.SeekStart) - this will allow us to read the file again from the beginning
	_, err = dst.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to seek to end of temp file", err)
		return
	}

	// Put the object into S3 using PutObject. You'll need to provide:
	// - The bucket name
	// - The file key. Use the same <random-32-byte-hex>.ext format as the key. e.g. 1a2b3c4d5e6f7890abcd1234ef567890.mp4
	// - The file contents (body). The temp file is an os.File which implements io.Reader
	// - Content type, which is the MIME type of the file.
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to generate random key", err)
		return
	}
	key := hex.EncodeToString(keyBytes)
	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(key),
		Body:        dst,
		ContentType: aws.String(mediaType),
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to upload video", err)
		return
	}

	// todo: Update the VideoURL of the video record in the database with the S3 bucket and key.
	// - S3 URLs are in the format https://<bucket-name>.s3.<region>.amazonaws.com/<key>. Make sure you use the correct region and bucket name!
	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, key)
	if err = cfg.db.UpdateVideo(database.Video{
		ID:                vid.ID,
		CreatedAt:         vid.CreatedAt,
		UpdatedAt:         time.Now(),
		ThumbnailURL:      vid.ThumbnailURL,
		VideoURL:          &videoURL,
		CreateVideoParams: vid.CreateVideoParams,
	}); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video", err)
		return
	}

	respondWithJSON(w, http.StatusCreated, map[string]any{
		"id":       vid.ID,
		"videoURL": &videoURL,
	})
}
