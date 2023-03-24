package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	routing "github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"

	"google.golang.org/api/drive/v3"
)

type GetFileRequest struct {
	FileUrl         string `json:"file_url"`
	DirectoryTarget string `json:"directory_target"`
}

type SuccessResponse struct {
	Message string `json:"message"`
}

type ExtractData struct {
	Status    bool
	ZipReader *zip.Reader
	TarReader *tar.Reader
}

func extractArchive(archive []byte, filetype string) ExtractData {
	response := ExtractData{Status: false}
	switch filetype {
	case "zip":
		reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			log.Println("extractArchive: Extract %s failed %s", filetype, err.Error)
			return response
		}
		response.Status = true
		response.ZipReader = reader
		return response
	case "tar.gz":
		uncompressedStream, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			log.Fatal("ExtractTarGz: NewReader failed")
		}

		reader := tar.NewReader(uncompressedStream)
		response.Status = true
		response.TarReader = reader
		return response
	}
	return response
}

func GetFfile(url string) ([]byte, bool) {
	resp, err := http.Get(url)
	if err != nil {
		return make([]byte, 0), false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return make([]byte, 0), false
	}

	return body, true
}

func UploadFile(fileBody bytes.Buffer, fileName string, dirName string) bool {
	ctx := context.Background()
	srv, err := drive.NewService(ctx)
	if err != nil {
		log.Fatal("Unable to access Drive API:", err)
		return false
	}

	res, err := srv.Files.Create(
		&drive.File{
			Parents: []string{dirName},
			Name:    fileName,
		},
	).Media(fileBody, googleapi.ChunkSize(int(len(fileBody)))).Do()
	if err != nil {
		log.Fatalln(err)
		return false
	}
	log.Println("UploadFile: upload file %s success %s", fileName, res.Id)

	// res2, err := srv.Permissions.Create(res.Id, &drive.Permission{
	//   Role: "reader",
	//   Type: "anyone",
	// }).Do()

	return true
}

func GetUploadFile(url string) bool {
	body, status := GetFfile(url)
	if !status {
		log.Println("GetUploadFile: File download failed")
		return false
	}

	urlArr := strings.Split(url, "/")
	archiveNameArr := strings.Split(urlArr[len(urlArr)-1], ".")
	dirName := archiveNameArr[0]

	archiveType := ""
	for i, s := range archiveNameArr[1:] {
		archiveType += s
		if i != len(archiveNameArr[1:])-1 {
			archiveType += "."
		}
	}
	log.Println("GetUploadFile: Download archive with type %s", archiveType)

	readerStruct := extractArchive(body, archiveType)

	if !readerStruct.Status {
		log.Println("GetUploadFile: Failed archive extract")
		return false
	}

	switch archiveType {
	case "zip":
		for _, file := range readerStruct.ZipReader.File {
			if file.FileInfo().IsDir() {
				continue
			}

			fileReader, err := file.Open()
			if err != nil {
				log.Println("GetUploadFile: File opening failed: %s %s", file.FileInfo(), err.Error)
				continue
			}

			var fileContents bytes.Buffer
			_, err = io.Copy(&fileContents, fileReader)
			if err != nil {
				log.Println("GetUploadFile: Failed copy file content in memory %s %s", file.FileInfo, err.Error)
				continue
			}

			go UploadFile(fileContents, file.Name, dirName)
		}
		return true
	case "tar.gz":
		for true {
			header, err := readerStruct.TarReader.Next()

			if err == io.EOF {
				break
			}

			if err != nil {
				log.Permission("ExtractTarGz: Next() failed: %s", err.Error())
				continue
			}

			switch header.Typeflag {
			case tar.TypeReg:
				var fileContents bytes.Buffer
				_, err = io.Copy(&fileContents, readerStruct.TarReader)
				if err != nil {
					log.Println("GetUploadFile: Failed copy file content in memory %s %s", file.FileInfo, err.Error)
					continue
				}

				go UploadFile(fileContents, header.Name, dirName)

			default:
				log.Println(
					"ExtractTarGz: uknown type: %s in %s",
					header.Typeflag,
					header.Name,
				)
			}

		}
		return true
	default:
		log.Println("GetUploadFile: Unknown archive type: %s", archiveType)
		return false
	}

	return true
}

func GetUploadHandler(c *routing.Context) {
	var requestBody GetFileRequest
	if err := json.Unmarshal(c.Request.Body(), &requestBody); err != nil {
		log.Println("GetUploadHandler: Decode request body failed: ", err.Error)
		c.Error("Invalid request body", fasthttp.StatusBadRequest)
		return
	}
	go GetUploadFile(requestBody.FileUrl)
	c.SetContentType("application/json")
	c.SetStatusCode(fasthttp.StatusOK)
	response, _ := json.Marshal(SuccessResponse{Message: "success"})
	c.Write(response)
}

func main() {
	router := routing.New()

	router.Post("/", func(c *routing.Context) error {
		GetUploadHandler(c)
		return nil
	})

	fmt.Println("Starting server...")
	panic(fasthttp.ListenAndServe(":8080", router.HandleRequest))
}
