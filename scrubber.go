package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
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
		if err != nil {
			return fmt.Errorf("(scrub) error reading row: %w", err)
		}
		row = r.Replace(row)
		_, err = io.WriteString(wr, row+"\n")
		if err != nil {
			return fmt.Errorf("(scrub) error writing row: %w", err)
		}
	}

	return nil
}

func noscrub(rd io.Reader, wr io.Writer) error {
	sc := bufio.NewReader(rd)
	for {
		row, err := sc.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("(noscrub) error reading row: %w", err)
		}
		_, err = io.WriteString(wr, row+"\n")
		if err != nil {
			return fmt.Errorf("(noscrub) error writing row: %w", err)
		}
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
		return fmt.Errorf("(scrubFile) error opening file: %w", err)
	}
	defer fr.Close()

	//check if the filename or path is in the scrubIndex.
	fdst, err = filePathScrub(fdst, si)
	if err != nil {
		return fmt.Errorf("(scrubFile) failed to scrub filename: %w", err)
	}

	//create folder to place file in
	os.MkdirAll(filepath.Dir(fdst), os.ModePerm)

	//create the file to place scrubbed data in
	fw, err := os.OpenFile(fdst, os.O_WRONLY+os.O_CREATE, os.ModePerm)
	if err != nil {
		log.Fatalf("open file error: %v", err)

	}
	defer fw.Close()

	return scrubStream(fr, fw, si, fsrc)
}

func scrubStream(fr io.Reader, fw io.Writer, si []string, fsrc string) error {

	ft := filepath.Ext(fsrc)

	switch ft {

	case ".zip":
		//write tempfile to determine filesize before streaming to tar archive
		tmp, err := ioutil.TempFile("", "vmw-scrubber-")
		if err != nil {
			return fmt.Errorf("tmpfile: %w", err)
		}
		defer os.Remove(tmp.Name())

		//copy zip file to tmp disk file
		i, err := io.Copy(tmp, fr)
		if err != nil {
			return fmt.Errorf("(scrubStream-zip) error copying .zip file to a tmp file: %w", err)
		}

		//open the zip file
		zr, err := zip.NewReader(tmp, i)
		if err != nil {
			return fmt.Errorf("(scrubStream-zip) error opening the tmp zile file: %w", err)
		}

		zw := zip.NewWriter(fw)
		defer zw.Close()

		for _, f := range zr.File {

			//check if the file is actually a dir
			if f.FileInfo().IsDir() {
				log.Printf("compressed/tar dir: %s", f.Name)
				//path := fmt.Sprintf("%s%c", f.Name, os.PathSeparator)

				_, err := zw.Create(f.Name)
				if err != nil {
					return fmt.Errorf("(srubStream-zip) could not create folder in zip archive: %w", err)
				}

				continue
			}

			//open the compressed file or archive
			fileInArchive, err := f.Open()
			if err != nil {
				return fmt.Errorf("(scrubStream-zip) error opening a file in the zip archive: %w", err)
			}

			newFileInArchive, err := zw.Create(f.Name)
			if err != nil {
				return fmt.Errorf("(scrubStream-zip) error creating a file in the new zip file: %w", err)
			}

			log.Printf("scrubbing zipped file: %s", f.Name)
			err = scrubStream(fileInArchive, newFileInArchive, si, f.Name)
			if err != nil {
				return fmt.Errorf("scrub: %w", err)
			}

			//close files when done
			fileInArchive.Close()

		}

	case ".gz", ".tgz":
		//gzip reader and writer

		gzr, err := gzip.NewReader(fr)
		if err != nil {
			return fmt.Errorf("couldnt create gzip reader: %w", err)
		}
		gzw := gzip.NewWriter(fw)

		//close file when ready
		defer gzw.Close()

		//if this is a tgz, append with .tar to ensure next loop catches case .tar
		if ft == ".tgz" {
			return scrubStream(gzr, gzw, si, strings.TrimSuffix(fsrc, ft)+".tar")
		}

		//if this is gz, just stream the file
		return scrubStream(gzr, gzw, si, strings.TrimSuffix(fsrc, ft))

	case ".tar":
		//tar reader and writer

		tarReader := tar.NewReader(fr)
		tarWriter := tar.NewWriter(fw)

		for {
			header, err := tarReader.Next()

			//if its the end of the archive, break
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("(scrubStream-tar) error opening the next file in archive: %w", err)
			}

			//check if the file is actually a dir
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

			log.Printf("scrubbing compressed/tar file: %s, byte: %d", header.Name, header.FileInfo().Size())

			//TODO: check if file is corrupt

			err = scrubStream(tarReader, tmp, si, header.Name)
			if err != nil {
				return fmt.Errorf("scrub: %w", err)
			}

			//get the filesize of the new file
			tmpinfo, err := tmp.Stat()
			if err != nil {
				return fmt.Errorf("couldnt get filesize: %w", err)
			}

			//update the header with the new filesize after scrubbing
			header.Size = tmpinfo.Size()
			tarWriter.WriteHeader(header)

			//since seeker is at end of the file, to copy the content we need to reset the seeker
			tmp.Seek(0, io.SeekStart)

			//copy the tmp file into the tar file
			_, err = io.Copy(tarWriter, tmp)
			if err != nil {
				return fmt.Errorf("couldnt copy file into tarfile: %w", err)
			}
		}

		//close the file
		tarWriter.Close()

		return scrub(fr, fw, si)

	case ".vmdk":
		log.Println("found vmdk file, just copying")
		return noscrub(fr, fw)

	case ".nvram":
		log.Println("found .nvram file, just copying")
		return noscrub(fr, fw)
	}

	return scrub(fr, fw, si)
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

// func fileType(f io.Reader) (string, []byte, error) {
// 	buf := make([]byte, 512)

// 	_, err := f.Read(buf)

// 	if err != nil && err != io.EOF {
// 		return "", buf, err
// 	}

// 	contentType := http.DetectContentType(buf)

// 	return contentType, buf, nil
// }

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
