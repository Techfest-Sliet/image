# image

## Webserver for image CDN

### Building
  - Install libvips
    For ArchLinux,
    ```sh
    # pacman -S libvips
    ```

    For Ubuntu, 
    ```sh
    # sudo add-apt-repository -y ppa:strukturag/libde265
    # sudo add-apt-repository -y ppa:strukturag/libheif
    # apt install libvips-dev
    ```
    - Build by
    ```sh
    $ go build
    ```

### Running
  ```sh
  ./image
  ```
