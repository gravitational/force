// _ is a meta helper function that switches parser
// to initialize complex nested structs 
_(
	kube.Job{
		Name: "my job",
		Spec.Template: _{
			Containers: []_{
				{
					Name: "",
					Image: "",
				},
			},
		},
	},
)
