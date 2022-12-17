package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"
)

func main() {

	//initiate index that will be used for scrubbing
	scrubIndex := []string{}

	//define flags, we want a starting directory, and the directory to output to
	inpath := flag.String("in", ".", "in")
	outpath := flag.String("out", "scrubbed", "out")
	custom := flag.String("custom", "custom.json", "custom")

	//parse for defined flags by user
	flag.Parse()

	//check if custom scrubIndex needs to be appended.
	if isFlagPassed("custom") {
		// Open our jsonfile
		jf, err := os.Open(*custom)
		// if we os.Open returns an error then handle it
		if err != nil {
			fmt.Println(err)
		}
		// defer the closing of our jsonfile so that we can parse it later on
		defer jf.Close()

		b, _ := ioutil.ReadAll(jf)

		var c []siRow

		json.Unmarshal([]byte(b), &c)
		spew.Dump(c)

		for i := 0; i < len(c); i++ {
			scrubIndex = append(scrubIndex, c[i].Readable, c[i].Anonymized)
		}
	}

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

	//generate the index.html to use as translation when talking to support
	err := generateIndexFile(scrubIndex)
	if err != nil {
		fmt.Println(err)
	}

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
