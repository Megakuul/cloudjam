package challenge

const InitCall = "init"

type InitInput struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

const RegisterResourceCall = "register_resource"

type CreateResource struct {
	Type    string `json:"type,omitempty"`
	Desired string `json:"desired,omitempty"`
}

type ReadResource struct {
	Type    string `json:"type,omitempty"`
	Desired string `json:"desired,omitempty"`
}

type UpdateResource struct {
	Type    string `json:"type,omitempty"`
	Desired string `json:"desired,omitempty"`
}

type DeleteResource struct {
	Type    string `json:"type,omitempty"`
	Desired string `json:"desired,omitempty"`
}
