package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/google/uuid"
	tfv1alpha2 "github.com/isaaguilar/terraform-operator/pkg/apis/tf/v1alpha2"
	"github.com/mattbaird/jsonpatch"
	admission "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecFactory  = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecFactory.UniversalDeserializer()
	jsonPatchType = admission.PatchTypeJSONPatch
	ctx           = context.TODO()
)

// Add kind AdmissionReview in scheme
func init() {
	_ = admission.AddToScheme(runtimeScheme)
}

type mutationHandler struct {
	serviceName string
}

func (f mutationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	admissionHandler(w, r, f.serviceName)
}

// admissionHandler handles the http portion of a request
func admissionHandler(w http.ResponseWriter, r *http.Request, serviceName string) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	// validate content type
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		log.Printf("contentType=%s, expect application/json", contentType)
		return
	}

	var responseObj runtime.Object
	obj, gvk, err := deserializer.Decode(body, nil, nil)
	if err != nil {
		msg := fmt.Sprintf("Request could not be decoded: %v", err)
		log.Println(msg)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	requestedAdmissionReview, ok := obj.(*admission.AdmissionReview)
	if !ok {
		log.Printf("Expected v1.AdmissionReview but got: %T", obj)
		return
	}

	responseAdmissionReview := &admission.AdmissionReview{}
	responseAdmissionReview.SetGroupVersionKind(*gvk)
	responseAdmissionReview.Response = mutate(*requestedAdmissionReview, serviceName)
	requestedAdmissionReview.Response = &admission.AdmissionResponse{}
	responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID
	responseObj = responseAdmissionReview

	respBytes, err := json.Marshal(responseObj)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(respBytes); err != nil {
		log.Println(err)
	}
}

// StartWebhook starts the webhook
func StartWebhook(tlsCertFilename, tlsKeyFilename, webhookURI, serviceName string) {
	h := mutationHandler{
		serviceName: serviceName,
	}
	server := http.NewServeMux()
	server.Handle(webhookURI, h)

	log.Println("Server started and listing on 8443...")
	err := http.ListenAndServeTLS(":8443", tlsCertFilename, tlsKeyFilename, server)
	log.Fatal(err)
}

func mutate(ar admission.AdmissionReview, serviceName string) *admission.AdmissionResponse {

	group := tfv1alpha2.SchemeGroupVersion.Group
	version := tfv1alpha2.SchemeGroupVersion.Version
	terraformResource := schema.GroupVersionResource{Group: group, Version: version, Resource: "terraforms"}
	if ar.Request.Resource != metav1.GroupVersionResource(terraformResource) {
		log.Printf("expect resource to be %s", terraformResource)
		return nil
	}

	raw := ar.Request.Object.Raw
	tf := tfv1alpha2.Terraform{}
	if _, kind, err := deserializer.Decode(raw, nil, &tf); err != nil {
		log.Println(err)
		log.Printf("Your kind is %s", kind)
		return &admission.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	// -- Mutations Start --

	log.Printf("Mutating %s", tf.Name)

	// generate UUID and secret name
	generatedUID := uuid.New()
	uuid := mutateOutputSecret(generatedUID.String())

	// Mutate OutputSecret
	tf.Spec.OutputsSecret = uuid

	tf.Annotations["mutation.galleybytes.com/mutated-by"] = serviceName

	// -- Mutations End --

	// Create and return JSON patch
	targetJSON, err := json.Marshal(tf)
	if err != nil {
		log.Println(err)
		return &admission.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	patch, err := jsonpatch.CreatePatch(raw, targetJSON)
	if err != nil {
		log.Println(err)
		return &admission.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	tfPatch, err := json.Marshal(patch)
	if err != nil {
		log.Println(err)
		return &admission.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}
	for _, p := range patch {
		log.Println(p)
	}
	return &admission.AdmissionResponse{Allowed: true, PatchType: &jsonPatchType, Patch: tfPatch}
}

func mutateOutputSecret(uid string) string {
	// Perform terraform secret mutation logic here
	uuid := uid + "-output-secret"

	return uuid
}
