package object

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	w "github.com/ibmjstart/cf-object-storage/writer"
	"github.com/ibmjstart/swiftlygo/auth"
)

// maxObjectSize is the largest size a file can be in Object Storage.
const maxObjectSize uint = 1000 * 1000 * 1000 * 5

// GetObjectInfo returns metadata for a given object.
func GetObjectInfo(dest auth.Destination, writer *w.ConsoleWriter, args []string) (string, error) {
	writer.SetCurrentStage("Fetching object info")

	container := args[3]
	object := args[4]

	objectInfo, headers, err := dest.(*auth.SwiftDestination).SwiftConnection.Object(container, object)
	if err != nil {
		return "", fmt.Errorf("Failed to get object %s: %s", object, err)
	}

	retval := fmt.Sprintf("\r%s%s\n\nName: %s\nContent type: %s\nSize: %d bytes\nLast modified: %s\n"+
		"Hash: %s\nIs pseudo dir: %t\nSubdirectory: \n%sHeaders:", w.ClearLine, w.Green("OK"),
		objectInfo.Name, objectInfo.ContentType, objectInfo.Bytes, objectInfo.ServerLastModified,
		objectInfo.Hash, objectInfo.PseudoDirectory, objectInfo.SubDir)
	for k, h := range headers {
		retval += fmt.Sprintf("\n\tName: %s Value: %s", k, h)
	}
	retval += fmt.Sprintf("\n")

	return retval, nil
}

// ShowObjects returns the names of all objects in a given container.
func ShowObjects(dest auth.Destination, writer *w.ConsoleWriter, args []string) (string, error) {
	writer.SetCurrentStage("Displaying objects")

	container := args[3]

	objects, err := dest.(*auth.SwiftDestination).SwiftConnection.ObjectNamesAll(container, nil)
	if err != nil {
		return "", fmt.Errorf("Failed to get objects: %s", err)
	}

	return fmt.Sprintf("\r%s%s\n\nObjects in container %s: %v\n", w.ClearLine, w.Green("OK"), container, objects), nil
}

// PutObject uploads an object to Object Storage.
func PutObject(dest auth.Destination, writer *w.ConsoleWriter, args []string) (string, error) {
	writer.SetCurrentStage("Uploading object")

	container := args[3]
	path := args[4]
	object := filepath.Base(path)

	if len(args) == 7 && args[5] == "-n" {
		object = args[6]
	}

	data, err := getFileContents(path)
	if err != nil {
		return "", fmt.Errorf("Failed to get file contents at path %s: %s", path, err)
	}

	hash := hashSource(data)

	// ADD SUPPORT FOR HEADERS
	objectCreator, err := dest.(*auth.SwiftDestination).SwiftConnection.ObjectCreate(container, object, true, hash, "", nil)
	if err != nil {
		return "", fmt.Errorf("Failed to create object: %s", err)
	}

	_, err = objectCreator.Write(data)
	if err != nil {
		return "", fmt.Errorf("Failed to write object: %s", err)
	}

	err = objectCreator.Close()
	if err != nil {
		return "", fmt.Errorf("Failed to close object writer: %s", err)
	}

	return fmt.Sprintf("\r%s%s\n\nUploaded object %s to container %s\n", w.ClearLine, w.Green("OK"), object, container), nil
}

// CopyObject copies an object from one container to another
func CopyObject(dest auth.Destination, writer *w.ConsoleWriter, args []string) (string, error) {
	writer.SetCurrentStage("Copying object")

	container := args[3]
	object := args[4]
	newContainer := args[5]

	_, err := dest.(*auth.SwiftDestination).SwiftConnection.ObjectCopy(container, object, newContainer, object, nil)
	if err != nil {
		return "", fmt.Errorf("Failed to copy object: %s", err)
	}

	return fmt.Sprintf("\r%s%s\n\nCopied object %s to container %s\n", w.ClearLine, w.Green("OK"), object, newContainer), nil
}

// GetObject downloads an object from object storage.
func GetObject(dest auth.Destination, writer *w.ConsoleWriter, args []string) (string, error) {
	writer.SetCurrentStage("Downloading object")

	container := args[3]
	objectName := args[4]
	destinationPath := args[5]

	object, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return "", fmt.Errorf("Failed to open/create object file: %s", err)
	}
	defer object.Close()

	_, err = dest.(*auth.SwiftDestination).SwiftConnection.ObjectGet(container, objectName, object, true, nil)
	if err != nil {
		return "", fmt.Errorf("Failed to get object %s: %s", objectName, err)
	}

	return fmt.Sprintf("\r%s%s\n\nDownloaded object %s to %s\n", w.ClearLine, w.Green("OK"), objectName, destinationPath), nil
}

// RenameObject renames a given object.
func RenameObject(dest auth.Destination, writer *w.ConsoleWriter, args []string) (string, error) {
	writer.SetCurrentStage("Renaming object")

	container := args[3]
	object := args[4]
	newName := args[5]

	_, err := dest.(*auth.SwiftDestination).SwiftConnection.ObjectCopy(container, object, container, newName, nil)
	if err != nil {
		return "", fmt.Errorf("Failed to rename object: %s", err)
	}

	err = dest.(*auth.SwiftDestination).SwiftConnection.ObjectDelete(container, object)
	if err != nil {
		return "", fmt.Errorf("Failed to delete object %s: %s", object, err)
	}

	return fmt.Sprintf("\r%s%s\n\nRenamed object %s to %s\n", w.ClearLine, w.Green("OK"), object, newName), nil
}

//DeleteObject removes a given object from Object Storage.
func DeleteObject(dest auth.Destination, writer *w.ConsoleWriter, args []string) (string, error) {
	var err error

	writer.SetCurrentStage("Deleting object")

	container := args[3]
	object := args[4]

	if len(args) == 6 && args[5] == "-l" {
		err = deleteLargeObject(dest, container, object)
	} else {
		err = deleteObject(dest, container, object)
	}
	if err != nil {
		return "", fmt.Errorf("Failed to delete object %s: %s", object, err)
	}

	return fmt.Sprintf("\r%s%s\n\nDeleted object %s from container %s\n", w.ClearLine, w.Green("OK"), object, container), nil
}

//deleteObject deletes a regular object.
func deleteObject(dest auth.Destination, container, objectName string) error {
	err := dest.(*auth.SwiftDestination).SwiftConnection.ObjectDelete(container, objectName)
	if err != nil {
		return fmt.Errorf("Failed to delete object %s: %s", objectName, err)
	}

	return nil
}

//deleteLargeObject deletes a large object, such as an SLO or DLO.
func deleteLargeObject(dest auth.Destination, container, objectName string) error {
	// Using the Open Stack Object Storage API directly as large object support is not
	// included in the ncw/swift library yet. There is an open pull request to merge the
	// large-object branch as of 11/22/16 at https://github.com/ncw/swift/pull/74.
	var client http.Client

	authUrl := dest.(*auth.SwiftDestination).SwiftConnection.StorageUrl
	authToken := dest.(*auth.SwiftDestination).SwiftConnection.AuthToken

	deleteUrl := authUrl + "/" + container + "/" + objectName + "?multipart-manifest=delete"

	request, err := http.NewRequest("DELETE", deleteUrl, nil)
	if err != nil {
		return fmt.Errorf("Failed to create request: %s", err)
	}
	request.Header.Set("X-Auth-Token", authToken)

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("Failed to make request: %s", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Failed to delete object with status %s", response.Status)
	}

	defer response.Body.Close()
	_, err = io.Copy(ioutil.Discard, response.Body)
	if err != nil {
		return fmt.Errorf("Failed to read response body: %s")
	}

	return nil
}

// getFileContents returns the raw contents of a file.
func getFileContents(sourcePath string) ([]byte, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("Failed to open source file: %s", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("Failed to get source file info: %s", err)
	}

	if uint(info.Size()) > maxObjectSize {
		return nil, fmt.Errorf("%s is too large to upload as a single object (max 5GB)", info.Name())
	}

	data := make([]byte, info.Size())
	_, err = file.Read(data)
	if err != nil {
		return nil, fmt.Errorf("Failed to read source file: %s", err)
	}

	return data, nil
}

// hashSource hashes raw file contents.
func hashSource(sourceData []byte) string {
	hashBytes := md5.Sum(sourceData)
	hash := hex.EncodeToString(hashBytes[:])

	return hash
}
