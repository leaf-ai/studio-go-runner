Use the following commands to download the installation script.  Before running it you must change the file to include password and Azure location information:
 
```shell
wget -O install_custom.sh https://raw.githubusercontent.com/leaf-ai/studio-go-runner/feature/233_kustomize/cloud/install.sh
```

Using vim for another file editing tool on linux to change the script to your needs, then run the script and use the echo command to give you the IP addresses of the server that were installed.

```shell
./install_custom.sh
```
