package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"text/template"

	t "github.com/achelovekov/n9k-modeling/templating"
)

func indexTemplating(w http.ResponseWriter, r *http.Request) {
	tpl.ExecuteTemplate(w, "indexTemplating.gohtml", nil)
}

func processTemplate(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Fatalln(err)
	}

	jsonData := []byte(r.FormValue("vars"))

	var serviceVariablesDB t.ServiceVariablesDB

	json.Unmarshal(jsonData, &serviceVariablesDB)
	fmt.Println(r.FormValue("serviceName"))
	fmt.Println(serviceVariablesDB)
}

var tpl *template.Template

func init() {
	tpl = template.Must(template.ParseGlob("templates/*.gohtml"))
}

func main() {
	http.HandleFunc("/indexTemplating", indexTemplating)
	http.HandleFunc("/processTemplate", processTemplate)
	http.ListenAndServe(":8080", nil)
}
