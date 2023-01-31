package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
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
	vips.Startup(&vips.Config{MaxCacheMem: (4 << 20)})
	r.Post("/save", handleSave)
	r.Get("/get", handleGet)
	r.Get("/", handleForm)
	s := &http.Server{
		Addr:           ":" + strconv.Itoa(*port),
		Handler:        r,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 10 << 20,
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
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("{error: \"Failure while allocating file\", fullError: \"" + err.Error() + "\" }"))
		log.Panicln(err)
	}
	imageId, err := saveImage(imageData, imageHeader, SAVE_PATH)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("{error: \"Failure while saving to file\", fullError: \"" + err.Error() + "\" }"))
		log.Panicln(err)
	}
	w.Write([]byte("{\"uuid\": \"" + imageId.String() + "\", \"message\": \"Success\"}"))
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
	imageData, imageMeta, err := image.ExportAvif(&vips.AvifExportParams{StripMetadata: true, Quality: 90, Speed: 6})
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
		imageName := SAVE_PATH + "Image-" + imageId.String() + ".avif"
		image, err := vips.NewImageFromFile(imageName)
		if err != nil {
			handleErr(w, http.StatusInternalServerError, "Couldn't open file", err)
			return
		}
		width, err := strconv.Atoi(q["width"][0])
		if err != nil {
			handleErr(w, http.StatusBadRequest, "Invalid width", err)
			return
		}
		//height, err := strconv.Atoi(q["height"][0])
		if err != nil {
			handleErr(w, http.StatusBadRequest, "Invalid height", err)
			return
		}
		err = image.Resize(float64(width) / float64(image.Width()), vips.KernelAuto)
		if err != nil {
			handleErr(w, http.StatusBadRequest, "Couldn't Resize Image", err)
			return
		}
		fmt.Printf("SIZE: {X: %d, Y: %d}\nSCALE: %f\n", image.Width(), image.Height(),float64(width) / float64(image.Width()) )
		imageData, _, err := image.ExportWebp(&vips.WebpExportParams{Quality: 80, ReductionEffort: 2})
		if err != nil {
			handleErr(w, http.StatusInternalServerError, "Couldn't Encode Image", err)
			return
		}
		w.Header().Set("Content-Type", "image/webp")
		w.Write(imageData)

	}
}

func handleErr(w http.ResponseWriter, code int, message string, err error) {
	w.WriteHeader(code)
	w.Write([]byte(fmt.Sprintf("{error: [\"%s: %s\", \"%s\"]}", http.StatusText(code), message, err.Error())))
}

func isImage(header *multipart.FileHeader) bool {
	return header.Header.Get("Content-Type")[0:6] == "image/"
}

func scale(x, y int, width, height float64) float64 {
	if x > y {
		return float64(x) / float64(width)
	}
	return float64(y) / float64(height)
}
