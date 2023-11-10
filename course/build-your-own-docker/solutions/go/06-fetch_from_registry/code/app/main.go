//go:build linux

package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	cp "github.com/otiai10/copy"
)

const DockerRegistryBaseURL = "https://registry.hub.docker.com"

// TODO Improvements:
//   - Watch the video on docker and implement other functionalities as well
//   - such as cgroups
//
// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	image := os.Args[2]
	command := os.Args[3]
	args := os.Args[4:]

	tempDir, err := createIsolation(command)
	// This might be incorrect, as we have chroot(ed) into that directory
	// Also, this is not required right now, as we're running on a container, and not exactly on local,
	// but iw would be handy in case we were doing local dev.
	// ! This command should not be run here, but rather after chrooting back to the main local fs.
	defer os.RemoveAll(tempDir)
	checkErr(err)

	// List the temp dir (root) to see all the files in it
	// printFilesAndDir("/")
	// printFilesAndDir("/bin")
	// catFile("/etc/resolv.conf")

	cmd := exec.Command(command, args...)

	// Wire up stdout and stderr from child process
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	// This is required because, if any one of them is nil, cmd.Run() will throw an error saying it
	// needs /dev/null in the new root
	cmd.Stdin = os.Stdin

	// Isolate the pid namespace, so that the processes running inside the containerised temp folder
	// cannot access the local/parent machine's process and make any destructive changes
	// CloneFlags is not available on Mac -- need to set: a couple of directives at the top of the file
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID,
	}

	// Fetch the image from docker registry to run the commands on
	d := &DockerRegistry{
		Image: image,
	}
	d.fetchImage()

	// Wire up exit codes from child process
	err = cmd.Run()
	if err != nil {
		fmt.Println("Error encountered in main function is: ", err.Error())
		if fmt.Sprintf("%T", err) == "*exec.ExitError" {
			os.Exit(err.(*exec.ExitError).ExitCode())
		}
	}
}

type DockerRegistry struct {
	Image string
	Token string
}

type DockerRegistryTokenResp struct {
	Token     string `json:"token"`
	ExpiresIn uint64 `json:"expires_in"`
	IssuedAt  string `json:"issued_at"`
}

type DockerImageLayers struct {
	MediaType string `json:"mediaType"`
	Size      int    `json:"size"`
	Digest    string `json:"digest"`
}

type DockerRegistryManifestResp struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Config        struct {
		MediaType string `json:"mediaType"`
		Size      int    `json:"size"`
		Digest    string `json:"digest"`
	} `json:"config"`
	Layers []DockerImageLayers `json:"layers"`
}

func httpRequest(url string, headers map[string]string) (*http.Response, error) {
	// fmt.Println("Url to make the request is: ", url)
	client := &http.Client{}
	req, _ := http.NewRequest("GET", url, nil)

	if headers != nil {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return resp, err
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		fmt.Printf("%s\n", string(b))

		return resp, fmt.Errorf("response code is not 200")
	}

	return resp, nil
}

func (d *DockerRegistry) fetchManifest() (DockerRegistryManifestResp, error) {
	url := fmt.Sprintf("%s/v2/library/%s/manifests/latest", DockerRegistryBaseURL, d.Image)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", d.Token),
		"Accept":        "application/vnd.docker.distribution.manifest.v2+json",
	}

	resp, err := httpRequest(url, headers)
	if err != nil {
		fmt.Println("Inside fetchManifest resp err check")

		return DockerRegistryManifestResp{}, err
	}

	var manifest DockerRegistryManifestResp
	if err = json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		fmt.Println("Inside fetchManifest json decode check")
		return DockerRegistryManifestResp{}, err
	}

	return manifest, nil
}

func (d *DockerRegistry) fetchToken() error {
	url := fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/%s:pull", d.Image)

	resp, err := httpRequest(url, nil)
	if err != nil {
		return err
	}

	var tokenResp DockerRegistryTokenResp
	if err = json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return err
	}
	d.Token = tokenResp.Token

	// fmt.Println("Token inside docker struct is: ", tokenResp.Token)

	return nil
}

func (d *DockerRegistry) downloadAndExtractLayers(layers []DockerImageLayers) error {
	for _, l := range layers {
		url := fmt.Sprintf("%s/v2/library/%s/blobs/%s", DockerRegistryBaseURL, d.Image, l.Digest)
		headers := map[string]string{
			"Authorization": fmt.Sprintf("Bearer %s", d.Token),
		}

		resp, err := httpRequest(url, headers)
		if err != nil {
			return err
		}

		data, _ := io.ReadAll(resp.Body)
		err = os.WriteFile("/image.tar.gz", data, 0755)
		checkErr(err)

		// Crude printing of the tar data
		// fmt.Println("Tar data is: ", string(data))

		// Since we are not setting the accept encoding header, http.Transport will automatically
		// set the header and transparently uncompress the gzip for us. Ref: https://go.dev/src/net/http/transport.go#L181
		// Use the shell's tar capability to extract, as Go does not have an easy and native
		// implementation

		// fmt.Println("Files before ungzip are: ")
		// printFilesAndDir("/")
		// fmt.Println("Files in /etc are: ")
		// printFilesAndDir("/etc")

		// Create a /dev/null file as it is needed by cmd.Run() command
		os.WriteFile("/dev/null", []byte{}, 0755)

		// UnGzip
		err = UnGzip("/image.tar.gz", "/image.tar")
		checkErr(err)

		// fmt.Println("Files after ungzip are: ")
		// printFilesAndDir("/")

		// Passing the root where we want to save the file
		// cmd := exec.Command("/bin/tar", "-xzf", "/image.tar.gz")
		// err = cmd.Run()
		// checkErr(err)

		err = Untar("image.tar", "image")
		checkErr(err)
		// Let's see if tarring was done correctly
		// fmt.Println("Files after untarring are: ")
		// printFilesAndDir("/")
		// fmt.Println("Files after untarring in image are: ")
		// printFilesAndDir("/image")

		// Copy back to root
		// err = cp.Copy("/image/", "/")
		// checkErr(err)

		// fmt.Println("Files after copying are: ")
		// printFilesAndDir("/")

		// A terrible idea/solution
		err = syscall.Chroot("image")
		checkErr(err)
	}

	return nil
}

func UnGzip(source, target string) error {
	reader, err := os.Open(source)
	if err != nil {
		return err
	}
	defer reader.Close()

	archive, err := gzip.NewReader(reader)
	if err != nil {
		return err
	}
	defer archive.Close()

	target = filepath.Join(target, archive.Name)
	writer, err := os.Create(target)
	if err != nil {
		return err
	}
	defer writer.Close()

	_, err = io.Copy(writer, archive)
	return err
}

func Untar(tarball, target string) error {
	// tarReader := tar.NewReader(r)
	reader, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		path := filepath.Join(target, header.Name)
		info := header.FileInfo()
		if info.IsDir() {
			if err = os.MkdirAll(path, info.Mode()); err != nil {
				return err
			}
			continue
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, tarReader)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DockerRegistry) fetchImage() error {
	// Do the auth dance
	err := d.fetchToken()
	if err != nil {
		return err
	}

	// Fetch the image manifest
	manifest, err := d.fetchManifest()
	if err != nil {
		panic(err)
	}

	// Download the layers
	err = d.downloadAndExtractLayers(manifest.Layers)
	if err != nil {
		panic(err)
	}

	return nil
}

// We want to ensure isolation from the local filesystem so that the command does not perform anything untoward or risky.
// So, we can create a temporary directory, make it root, copy the executable and run the same
func createIsolation(executable string) (string, error) {
	const resolvConfPath = "/etc/resolv.conf"
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "docker-codecrafters")

	if err != nil {
		return "", err
	}

	// Print the files and folders
	// fmt.Println("Files unser bin are: ")
	// printFilesAndDir("/bin")
	// fmt.Println("Files under /usr are: ")
	// printFilesAndDir("/usr")
	// fmt.Println("Files under /usr/bin are: ")
	// printFilesAndDir("/usr/bin")

	// Create the required directories in the temp dir
	// os.MkdirAll(tempDir+"/bin", os.ModeAppend)
	os.MkdirAll(tempDir+filepath.Dir(executable), os.ModeAppend)
	// Might need /etc/resolve.conf to make DNS requests from inside the container, but is this the right thing to do?
	os.MkdirAll(tempDir+"/etc", os.ModeAppend)
	os.MkdirAll(tempDir+"/dev", os.ModeAppend)
	// os.MkdirAll(tempDir+"/usr/bin", os.ModeAppend)

	// Let's copy the executable first
	copy(executable, tempDir+executable)
	// Set the correct permissions
	if err = os.Chmod(tempDir+executable, 0755); err != nil {
		return tempDir, err
	}

	cp.Copy("/etc", tempDir+"/etc")
	// cp.Copy("/bin", tempDir+"/bin")
	// copy("/bin/tar", tempDir+"/bin/tar")
	// if err = os.Chmod(tempDir+"/bin/tar", 0755); err != nil {
	// 	return tempDir, err
	// }

	// Move to the temp dir
	syscall.Chdir(tempDir)
	if err != nil {
		return tempDir, err
	}
	// Make temp dir as the root
	err = syscall.Chroot(tempDir)
	if err != nil {
		return tempDir, err
	}

	return tempDir, nil
}

func printFilesAndDir(path string) {
	files, err := os.ReadDir(path)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		fmt.Println(file.Name(), file.IsDir())
	}
}

func catFile(path string) {
	data, _ := os.ReadFile(path)
	fmt.Printf("File contents for path: %s, are: %s\n", path, string(data))
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func copy(src string, dst string) {
	// Read all content of src to data, may cause OOM for a large file.
	data, err := os.ReadFile(src)
	checkErr(err)
	// Write data to dst
	err = os.WriteFile(dst, data, 0644)
	checkErr(err)
}
