# vmw-logscrubber

---

Disclaimer: This is not an offical VMware log scrubber! This scrubber comes with no warranty or guarantee and it's your own responsiblity to verify that all sensitive informaton has been removed before sharing the logs.

---

This is a logscrubber that will consume MOREF (Managed Object REFerence) data from vCenter and anonymize logs.
Sensitive information such as hostnames and other identifyable data will be replaced with a generic ID.
An example would be the hostname sfo01-w01-esx01.rainpole.io, which would be replaced in a logfile with it's corresponding MOREF 'host-19'.

"This is an example string sfo01-w01-esx01.rainpole.io" would become "This is an example string host-19"
