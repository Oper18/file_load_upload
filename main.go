package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/nwaples/rardecode"
	routing "github.com/qiangxue/fasthttp-routing"
	"github.com/ulikunitz/xz"
	"github.com/valyala/fasthttp"
)

type GetFileRequest struct {
	FileUrl         string `json:"file_url"`
	DirectoryTarget string `json:"directory_target"`
}

type SuccessResponse struct {
	Message string `json:"message"`
}

type ExtractData struct {
	Status       bool
	ZipReader    *zip.Reader
	TarReader    *tar.Reader
	RarReader    *rardecode.Reader
	SevenZReader *xz.Reader
	SimpleFile   *bytes.Buffer
}

func extractArchive(archive []byte, filetype string) ExtractData {
	response := ExtractData{Status: false}
	switch filetype {
	case "zip":
		reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			log.Println("extractArchive: Zip NewReader %s failed %s", filetype, err.Error)
			return response
		}
		response.Status = true
		response.ZipReader = reader
		log.Println("extractArchive: Zip NewReader success")
		return response
	case "tar.gz":
		uncompressedStream, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			log.Println("extractArchive: TarGz NewReader failed: ", filetype, err.Error)
			return response
		}

		reader := tar.NewReader(uncompressedStream)
		response.Status = true
		response.TarReader = reader
		log.Println("extractArchive: TarGz NewReader success")
		return response
	case "rar":
		reader := bytes.NewReader(archive)
		decoder, err := rardecode.NewReader(reader, "")
		if err != nil {
			log.Println("extractArchive: Rar NewReader failed: ", filetype, err.Error)
			return response
		}
		response.Status = true
		response.RarReader = decoder
		log.Println("extractArchive: Rar NewReader success")
		return response
	case "7z":
		archiveFile := bytes.NewReader(archive)
		decoder, err := xz.NewReader(archiveFile)
		if err != nil {
			log.Println("extractArchive: 7z NewReader failed: ", filetype, err.Error)
			return response
		}
		tarReader := tar.NewReader(decoder)
		response.Status = true
		response.TarReader = tarReader
		log.Println("extractArchive: 7z NewReader success")
		return response
	default:
		response.Status = true
		response.SimpleFile = bytes.NewBuffer(archive)
		log.Println("extractArchive: File return success")
		return response
	}
	return response
}

func GetFfile(url string) ([]byte, bool) {
	resp, err := http.Get(url)
	if err != nil {
		log.Println("GetFfile: Failed file download %s", err.Error)
		return make([]byte, 0), false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("GetFfile: Failed file reading %s", err.Error)
		return make([]byte, 0), false
	}

	return body, true
}

func UploadFile(fileBody *bytes.Buffer, fileName string, dirName string) bool {
	// ctx := context.Background()
	// srv, err := drive.NewService(ctx)
	// if err != nil {
	//   log.Fatal("Unable to access Drive API:", err)
	//   return false
	// }
	//
	// fileReader := bytes.NewReader(fileBody.Bytes())
	//
	// res, err := srv.Files.Create(
	//   &drive.File{
	//     Parents: []string{dirName},
	//     Name:    fileName,
	//   },
	// ).Media(fileReader, googleapi.ChunkSize(int(fileBody.Len()))).Do()
	// if err != nil {
	//   log.Fatalln(err)
	//   return false
	// }
	// log.Println("UploadFile: upload file %s success %s", fileName, res.Id)
	log.Println("UploadFile: upload file %s success %s", fileName, "1")

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

			go UploadFile(&fileContents, file.Name, dirName)
		}
		return true
	case "tar.gz", "7z":
		for true {
			header, err := readerStruct.TarReader.Next()

			if err == io.EOF {
				break
			}

			if err != nil {
				log.Println("ExtractTarGz: Next() failed: %s", err.Error())
				continue
			}

			switch header.Typeflag {
			case tar.TypeReg:
				var fileContents bytes.Buffer
				_, err = io.Copy(&fileContents, readerStruct.TarReader)
				if err != nil {
					log.Println("GetUploadFile: Failed copy file content in memory %s %s", header.Name, err.Error)
					continue
				}

				go UploadFile(&fileContents, header.Name, dirName)

			default:
				log.Println(
					"ExtractTarGz: uknown type: %s in %s",
					header.Typeflag,
					header.Name,
				)
			}

		}
		return true
	case "rar":
		for {
			header, err := readerStruct.RarReader.Next()
			if err != nil {
				break
			}

			if header.IsDir {
				continue
			}

			var fileContents bytes.Buffer
			_, err = io.Copy(&fileContents, readerStruct.RarReader)
			if err != nil {
				log.Println("GetUploadFile: Failed copy file content in memory %s %s", header.Name, err.Error)
				continue
			}

			go UploadFile(&fileContents, header.Name, dirName)

			return true
		}
	// case "7z":
	//   for {
	//     header, err := readerStruct.SevenZReader.Next()
	//     if err != nil {
	//       if err == io.EOF {
	//         break
	//       }
	//       log.Println("GetUploadFile: Failed copy file content in memory %s %s", header.Name, err.Error)
	//     }
	//
	//     if header.IsDir() {
	//       continue
	//     }
	//     var fileContents bytes.Buffer
	//     _, err = io.Copy(&fileContents, readerStruct.SevenZReader)
	//     if err != nil {
	//       log.Println("GetUploadFile: Failed copy file content in memory %s %s", header.Name, err.Error)
	//       continue
	//     }
	//
	//     go UploadFile(&fileContents, header.Name, dirName)
	//   }
	//   return true
	default:
		go UploadFile(readerStruct.SimpleFile, dirName, "/")
		return true
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
	panic(fasthttp.ListenAndServe(":8088", router.HandleRequest))
}
