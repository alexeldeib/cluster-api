module sigs.k8s.io/cluster-api

go 1.12

require (
	github.com/Azure/go-autorest/autorest v0.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/gophercloud/gophercloud v0.2.0 // indirect
	github.com/onsi/ginkgo v1.7.0
	github.com/onsi/gomega v1.4.3
	github.com/pkg/errors v0.8.1
	github.com/sergi/go-diff v1.0.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.3
	golang.org/x/net v0.0.0-20190311183353-d8887717615a
	k8s.io/api v0.0.0-20190614205929-e4e27c96b39a
	k8s.io/apimachinery v0.0.0-20190612125636-6a5db36e93ad
	k8s.io/apiserver v0.0.0-20190615170205-3722cb685593
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/component-base v0.0.0-20190617074208-2b0aae80ca81
	k8s.io/klog v0.3.1
	k8s.io/utils v0.0.0-20190506122338-8fab8cb257d5
	sigs.k8s.io/controller-runtime v0.2.0-beta.2
)

replace (
	k8s.io/api => k8s.io/api v0.0.0-20190409021203-6e4e0e4f393b
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go => k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
)
