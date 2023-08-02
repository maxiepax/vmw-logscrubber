---
Disclaimer: This is not an offical VMware log scrubber! This scrubber comes with no warranty or guarantee and it's your own responsiblity to verify that all sensitive information has been removed before sharing the logs.
---

# vmw-logscrubber

This is a logscrubber that consumes MoREF (Managed Object REFerence) data from vCenter and anonymizes logs.
Sensitive information such as hostnames and other identifyable data will be replaced with a generic ID.
An example would be the hostname sfo01-w01-esx01.rainpole.io, which would be replaced in a logfile with it's corresponding MoREF 'host-19'.

"This is an example string sfo01-w01-esx01.rainpole.io" would become "This is an example string host-19"

## security

This logscrubber is intended to be used by corporations or organisations that have such high security that they
can't send unwashed loggs to support or 3rd party. Because of this i recommend that you clone, and compile your
own binary to guarantee that the compiled binary contains only what you expect. The source code is written with
as few external libraries as possible to make it as easy as possible to review the code to ensure that nothing
malicious is contained in the code.

- main.go contains just the calls to the functions that perform the connection to vCenter, and scrubbing of logs.
- scrubber.go contains the functions that perform the actual scrubbing of files.
- vmw.go contains the functions required for the connection to vCenter and retrieval of MoREF entrys.

For the convenience of those who don't want to compile their own binary, binaries are available. (soon)

## running

When you have your log bundle available, there is no need to unzip/untar any content, the tool will "dig" through a unlimited
amount of nested tgz files to scrub the data.

Flags available are: <br/>
-in "in-directory" # this is the directory of the logs you want to scrub.<br/>
-out "out-directory" # this is a directory that will be created, and where your scrubbed logs will end up.<br/>
-custom "jsonfile.json" (default custom.json) # if you want to add more key->value's to scrub other than MoREF, please see the included example json file.<br/>
-url # if you don't want to use environment variables for the information required to connect to vCenter, use url with the format `-url username:password@vcenter.rainpole.io` <br/>
For securitys sake, add a prepending space before the command to avoid bash saving the password to it's history.<br/>
e.g instead of `./vmw-scrubber -vsphere -url` write ` ./vmw-scrubber -vsphere -url`

Environment variables:
for security, if you dont want to store usernames and passwords in plain text, use the follow environment varaibles.

export GOVMOMI_URL=https://vcenter.rainpole.io

export GOVMOMI_USERNAME=admin-username

export GOVMOMI_PASSWORD=admin-password

to excute, run: `./vmw-logscrubber -in ./logs -out ./scrubbed-logs -custom custom.json`

## feedback

Companies and Organisations have different requirements when it comes to scrubbing logs. I implement the things that i can think of
that would be useful, but only you know whats important to you! If you have any feedback on missing features, or improvements, please do tell!

## TODO:

- create understanding for other filetypes then text and tgz, certain files can't be scrubbed.
- ~~create the translation table, will be a .html file that is not included in scrubbed data so that you can translate when support refers to a MoREF when you need to know the underlaying human readable item.~~
- add css to index so that it's easier to read.
- implement support for regex, although slower and tricky to get right, i understand there will be scenarios where you need to dynamically look for strings.
- currently scrubbs most vCenter/ESXi data. Want to ensure vSAN / NSX-T information is properly scrubbed.
