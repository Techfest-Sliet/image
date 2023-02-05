package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	//"github.com/qiniu/qmgo"
	"html/template"
	"mime/multipart"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/gernest/alien"
	"github.com/google/uuid"
)

var (
	port = flag.Int("port", 8080, "The port the webserver will listen on")
)

func main() {
	flag.Parse()
	log.Println("Initializing the router!")
	r := alien.New()
	vips.Startup(&vips.Config{MaxCacheMem: (256 << 20)})
	r.Post("/save", handleSave)
	r.Get("/get", handleGet)
	r.Get("/", handleForm)
	s := &http.Server{
		Addr:           ":" + strconv.Itoa(*port),
		Handler:        r,
		ReadTimeout:    100 * time.Second,
		WriteTimeout:   100 * time.Second,
		MaxHeaderBytes: 20 << 20,
	}
	log.Fatal(s.ListenAndServe())
}

const SAVE_PATH = "images/"

func handleForm(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("template.html"))
	tmpl.Execute(w, struct{ Success bool }{true})
}

func handleSave(w http.ResponseWriter, r *http.Request) {
	imageData, imageHeader, err := r.FormFile("image")
	w.Header().Set("Content-Type", "application/json")
	if isImage(imageHeader) {
		log.Println(imageHeader.Header.Get("Content-Type"))
		log.Println(isSVG(imageHeader))
		if isSVG(imageHeader) {
			imageId, err := saveSVG(imageData, imageHeader, SAVE_PATH)
			if err != nil {
				handleErr(w, http.StatusInternalServerError, "Failure while saving to file", err)
				log.Panicln(err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("{\"uuid\": \"" + imageId.String() + "\", \"message\": \"Success\"}"))
			return
		}
		if err != nil {
			handleErr(w, http.StatusInternalServerError, "Failure while allocating file", err)
			log.Panicln(err)
		}
		imageId, err := saveImage(imageData, imageHeader, SAVE_PATH)
		if err != nil {
			handleErr(w, http.StatusInternalServerError, "Failure while saving to file", err)
			log.Panicln(err)
		}
		w.Write([]byte("{\"uuid\": \"" + imageId.String() + "\", \"message\": \"Success\"}"))
	} else {
		handleErr(w, http.StatusBadRequest, "Is not an image", errors.New("The mimetype of the given file does not match image/*"))
	}
}

func saveSVG(data io.Reader, header *multipart.FileHeader, savePath string) (uuid.UUID, error) {
	log.Println("Reading the file")
	imageId := uuid.New()
	svgFile, err := os.Create(savePath + "Image-" + imageId.String() + ".svg")
	if err != nil {
		return uuid.Nil, err
	}
	_, err = io.Copy(svgFile, data)
	if err != nil {
		return uuid.Nil, err
	}
	return imageId, nil
}

func saveImage(data io.Reader, header *multipart.FileHeader, savePath string) (uuid.UUID, error) {
	log.Println("Reading the file")
	image, err := vips.NewImageFromReader(data)
	if err != nil {
		return uuid.Nil, err
	}
	image.OptimizeICCProfile()
	if err != nil {
		return uuid.Nil, err
	}
	log.Println("Starting the export")
	imageData, imageMeta, err := image.ExportAvif(&vips.AvifExportParams{StripMetadata: true, Quality: 90, Speed: 5})
	if err != nil {
		return uuid.Nil, err
	}
	log.Println("Writing the file")
	imageId := uuid.New()
	err = ioutil.WriteFile(savePath+"Image-"+imageId.String()+imageMeta.Format.FileExt(), imageData, 0644)
	if err != nil {
		return uuid.Nil, err
	}
	return imageId, nil
}

func handleGet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	requiredFields := []string{"uuid", "width", "height"}
	for i := 0; i < len(requiredFields); i++ {
		if !q.Has(requiredFields[i]) {
			handleErr(w, http.StatusBadRequest, "Missing Fields", errors.New("Missing Fields: "+requiredFields[i]))
			return
		}
		imageId, err := uuid.Parse(q["uuid"][0])
		if err != nil {
			handleErr(w, http.StatusBadRequest, "Invalid UUID", err)
			return
		}
		imageName := SAVE_PATH + "Image-" + imageId.String()

		//if _, err := os.Stat(imageName + ".avif"); err == nil {
		imageName += ".avif"
		fmt.Println("AVIF File exists")
		image, err := vips.NewImageFromFile(imageName)
		if err != nil {
			handleErr(w, http.StatusInternalServerError, "Couldn't open file", err)
			return
		}
		width, height := image.Width(), image.Height()
		if len(q["width"]) > 0 {
			width, err = strconv.Atoi(q["width"][0])
			if err != nil || width < 1 {
				handleErr(w, http.StatusBadRequest, "Invalid width", err)
				return
			}
		}
		if len(q["height"]) > 0 {
			height, err = strconv.Atoi(q["height"][0])
			if err != nil || height < 1 {
				handleErr(w, http.StatusBadRequest, "Invalid height", err)
				return
			}
		}
		err = image.Resize(scale(width, height, image.Width(), image.Height()), vips.KernelAuto)
		if err != nil {
			handleErr(w, http.StatusBadRequest, "Couldn't Resize Image", err)
			return
		}
		fmt.Printf("SIZE: {X: %d, Y: %d}\nSCALE: %f\n", image.Width(), image.Height(), float64(width)/float64(image.Width()))
		err = image.ExtractArea((image.Width()-width)/2, (image.Height()-height)/2, width, height)
		if err != nil {
			handleErr(w, http.StatusBadRequest, "Couldn't Crop Image", err)
			return
		}
		fmt.Printf("SIZE: {X: %d, Y: %d}\nSCALE: %f\n", image.Width(), image.Height(), float64(width)/float64(image.Width()))
		imageData, _, err := image.ExportWebp(&vips.WebpExportParams{Quality: 80, ReductionEffort: 2})
		if err != nil {
			handleErr(w, http.StatusInternalServerError, "Couldn't Encode Image", err)
			return
		}
		w.Header().Set("Content-Type", "image/webp")
		w.Write(imageData)
		/*
				} else {
					fmt.Printf("AVIF File does not exist\n")
					if _, err := os.Stat(imageName + ".svg"); err == nil {
						imageName += ".svg"
						svgData, err := ioutil.ReadFile(imageName)
						if err != nil {
							handleErr(w, http.StatusInternalServerError, "Couldn't open file", err)
							return
						}
						w.Header().Set("Content-Type", "image/svg+xml")
						w.Write(svgData)
						fmt.Printf("SVG File exists\n")
					} else {
						fmt.Printf("Image does not exist\n")
						handleErr(w, http.StatusInternalServerError, "Couldn't open file", errors.New("Specified image does not exist"))
						return
					}
		}*/

	}
}

func handleErr(w http.ResponseWriter, code int, message string, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write([]byte(fmt.Sprintf("{\"error\": [\"%s: %s\", \"%s\"]}", http.StatusText(code), message, err.Error())))
}

func isImage(header *multipart.FileHeader) bool {
	return header.Header.Get("Content-Type")[0:6] == "image/"
}

func isSVG(header *multipart.FileHeader) bool {
	return header.Header.Get("Content-Type")[0:9] == "image/svg"
}

func scale(x, y, width, height int) float64 {
	return float64(max(x,y))/float64(min(width, height))
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}
