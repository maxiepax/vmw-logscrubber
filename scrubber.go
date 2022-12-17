package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func scrub(rd io.Reader, wr io.Writer, si []string) error {
	//load string
	r := strings.NewReplacer(si...)

	sc := bufio.NewReader(rd)
	for {
		row, err := sc.ReadString('\n')
		if err == io.EOF {
			break
		}
		row = r.Replace(row)
		_, err = io.WriteString(wr, row+"\n")
		if err != nil {
			return err
		}
	}
	//if err := row.Err(); err != nil {
	//	return err
	//}

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

	case "application/octet-stream":
		log.Println("cant scrub binary files")
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

type siRow struct {
	Readable   string `json:"readable"`
	Anonymized string `json:"anonymized"`
}

func isFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func generateIndexFile(si []string) error {
	f, err := os.OpenFile("index.html", os.O_WRONLY+os.O_CREATE, os.ModePerm)
	if err != nil {
		log.Fatalf("open file error: %v", err)

	}
	defer f.Close()

	_, err = io.WriteString(f, "<html> <table> <tr> <th> Human Readable </th> <th> Anonymized </th> </tr> \n")
	if err != nil {
		return fmt.Errorf("failed to write html header: %w", err)
	}

	for k, v := range si {
		if k%2 == 0 {
			_, err = io.WriteString(f, "<tr><td>"+v+"</td>\n")
			if err != nil {
				return err
			}
		} else {
			_, err = io.WriteString(f, "<td>"+v+"</td></tr>\n")
			if err != nil {
				return err
			}
		}
	}

	_, err = io.WriteString(f, "</table> </html> \n")
	if err != nil {
		return fmt.Errorf("failed to write html footer: %w", err)
	}

	return nil
}
