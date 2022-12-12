package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/vmware/govmomi/session/cache"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

func scrub(rd io.Reader, wr io.Writer, si []string) error {
	//load string
	r := strings.NewReplacer(si...)

	sc := bufio.NewScanner(rd)
	for sc.Scan() {
		row := sc.Text()
		row = r.Replace(row)
		_, err := io.WriteString(wr, row+"\n")
		if err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}

	return nil
}

func filePathScrub(fdst string, si []string) (string, error) {
	//load string
	r := strings.NewReplacer(si...)

	fdst = r.Replace(fdst)

	return fdst, nil
}

func scrubFile(fsrc, fdst string, si []string) error {

	log.Printf("scrubbing file: %s", fsrc)
	//can we open the file?
	fr, err := os.OpenFile(fsrc, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return err
	}
	defer fr.Close()

	//check if the filename or path is in the scrubIndex.
	fdst, err = filePathScrub(fdst, si)
	if err != nil {
		return fmt.Errorf("filename scrub: %w", err)
	}

	//create folder to place file in
	os.MkdirAll(filepath.Dir(fdst), os.ModePerm)

	//create the file to place scrubbed data in
	fw, err := os.OpenFile(fdst, os.O_WRONLY+os.O_CREATE, os.ModePerm)
	if err != nil {
		log.Fatalf("open file error: %v", err)

	}
	defer fw.Close()

	return scrubStream(fr, fw, si)
}

func scrubStream(fr io.Reader, fw io.Writer, si []string) error {

	//check the file type
	ct, head, err := fileType(fr)
	if err != nil {
		return fmt.Errorf("filetype: %w", err)
	}

	//since tarreader doesn't have a seek function, we need to merge the 512 byte used to detect filetype, with the file again
	r := io.MultiReader(bytes.NewReader(head), fr)

	switch ct {
	case "application/x-gzip":

		//gzip reader and writer
		gzr, err := gzip.NewReader(r)
		if err != nil {
			return err
		}
		gzw := gzip.NewWriter(fw)

		//tar reader and writer
		tarReader := tar.NewReader(gzr)
		tarWriter := tar.NewWriter(gzw)

		for {
			header, err := tarReader.Next()

			if err == io.EOF {
				break
			}

			if err != nil {
				return err
			}

			if header.FileInfo().IsDir() {
				log.Printf("compressed/tar dir: %s", header.Name)
				continue
			}

			//write tempfile to determine filesize before streaming to tar archive
			tmp, err := ioutil.TempFile("", "vmw-scrubber-")
			if err != nil {
				return fmt.Errorf("tmpfile: %w", err)
			}
			defer os.Remove(tmp.Name())

			log.Printf("scrubbing compressed/tar file: %s", header.Name)
			err = scrubStream(tarReader, tmp, si)
			if err != nil {
				return fmt.Errorf("scrub: %w", err)
			}

			//get the filesize of the new file
			tmpinfo, err := tmp.Stat()
			if err != nil {
				return fmt.Errorf("stat: %w", err)
			}

			//update the header with the new filesize after scrubbing
			header.Size = tmpinfo.Size()

			tarWriter.WriteHeader(header)

			//since seeker is at end of the file, to copy the content we need to reset the seeker
			tmp.Seek(0, io.SeekStart)

			//copy the tmp file into the tar file
			_, err = io.Copy(tarWriter, tmp)
			if err != nil {
				return err
			}
		}

		//close the files
		tarWriter.Close()
		gzw.Close()

	}

	return scrub(r, fw, si)

}

func buildFileList(path string) []string {
	r := []string{}
	err := filepath.Walk(path,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			//only store files, no dirs.
			if !info.IsDir() {
				r = append(r, path)
			}
			return nil
		})
	if err != nil {
		log.Println(err)
	}

	return r
}

func fileType(f io.Reader) (string, []byte, error) {
	buf := make([]byte, 512)

	_, err := f.Read(buf)

	if err != nil && err != io.EOF {
		return "", buf, err
	}

	contentType := http.DetectContentType(buf)

	return contentType, buf, nil
}

// getEnvString returns string from environment variable.
func getEnvString(v string, def string) string {
	r := os.Getenv(v)
	if r == "" {
		return def
	}

	return r
}

// getEnvBool returns boolean from environment variable.
func getEnvBool(v string, def bool) bool {
	r := os.Getenv(v)
	if r == "" {
		return def
	}

	switch strings.ToLower(r[0:1]) {
	case "t", "y", "1":
		return true
	}

	return false
}

const (
	envURL      = "GOVMOMI_URL"
	envUserName = "GOVMOMI_USERNAME"
	envPassword = "GOVMOMI_PASSWORD"
	envInsecure = "GOVMOMI_INSECURE"
)

var urlDescription = fmt.Sprintf("ESX or vCenter URL [%s]", envURL)
var urlFlag = flag.String("url", getEnvString(envURL, ""), urlDescription)

var insecureDescription = fmt.Sprintf("Don't verify the server's certificate chain [%s]", envInsecure)
var insecureFlag = flag.Bool("insecure", getEnvBool(envInsecure, false), insecureDescription)

func processOverride(u *url.URL) {
	envUsername := os.Getenv(envUserName)
	envPassword := os.Getenv(envPassword)

	// Override username if provided
	if envUsername != "" {
		var password string
		var ok bool

		if u.User != nil {
			password, ok = u.User.Password()
		}

		if ok {
			u.User = url.UserPassword(envUsername, password)
		} else {
			u.User = url.User(envUsername)
		}
	}

	// Override password if provided
	if envPassword != "" {
		var username string

		if u.User != nil {
			username = u.User.Username()
		}

		u.User = url.UserPassword(username, envPassword)
	}
}

// NewClient creates a vim25.Client for use in the examples
func NewClient(ctx context.Context) (*vim25.Client, error) {
	// Parse URL from string
	u, err := soap.ParseURL(*urlFlag)
	if err != nil {
		return nil, err
	}

	// Override username and/or password as required
	processOverride(u)

	// Share govc's session cache
	s := &cache.Session{
		URL:      u,
		Insecure: *insecureFlag,
	}

	c := new(vim25.Client)
	err = s.Login(ctx, c, nil)
	if err != nil {
		return nil, err
	}

	return c, nil
}

// Run calls f with Client create from the -url flag if provided,
// otherwise runs the example against vcsim.
func Run(f func(context.Context, *vim25.Client) error) {

	var err error
	var c *vim25.Client

	ctx := context.Background()
	c, err = NewClient(ctx)
	if err == nil {
		err = f(ctx, c)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func main() {

	//initiate index that will be used for scrubbing
	scrubIndex := []string{}

	//define flags, we want a starting directory, and the directory to output to
	inpath := flag.String("in", ".", "in")
	outpath := flag.String("out", "scrubbed", "out")
	//custom := flag.String("custom", "", "custom")

	//parse for defined flags by user
	flag.Parse()

	//connect to vCenter to obtain MoReF Objects.
	Run(func(ctx context.Context, c *vim25.Client) error {
		// Create a view of Network types
		m := view.NewManager(c)

		v, err := m.CreateContainerView(ctx, c.ServiceContent.RootFolder, nil, true)
		if err != nil {
			log.Fatal(err)
		}

		var content []types.ObjectContent

		//retrieve all MoReFs availble in vCenter.
		err = v.Retrieve(ctx, nil, []string{"name"}, &content)
		if err != nil {
			return err
		}

		//check length of reponse.
		l := len(content)
		fmt.Printf("Retrieved %d entrys from vCenter \n", l)

		//iterate through response and push into scrubIndex
		for _, item := range content {
			s := strings.Split(item.Obj.String(), ":")
			scrubIndex = append(scrubIndex, item.PropSet[0].Val.(string), s[1])
		}

		return nil
	})

	//check if custom searchIndex needs to be added
	//TODO:

	//build a list of files that need to be scrubbed
	list := buildFileList(*inpath)

	//itterate through the array of files and folders
	for i := 0; i < len(list); i++ {
		err := scrubFile(list[i], filepath.Join(*outpath, strings.TrimPrefix(list[i], *inpath)), scrubIndex)
		if err != nil {
			log.Println(err)
		}
	}

}
