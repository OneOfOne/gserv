package router

import (
	"encoding/json"
	"strings"
)

type Swagger struct {
	OpenAPI string          `json:"openAPI,omitempty" yaml:"openAPI,omitempty"`
	Server  []SwaggerServer `json:"server,omitempty" yaml:"server,omitempty"`
	Info    *SwaggerInfo    `json:"info,omitempty" yaml:"info,omitempty"`
	Paths   SwaggerPath     `json:"paths,omitempty" yaml:"paths,omitempty"`
}

type SwaggerInfo struct {
	Title       string `json:"title,omitempty" yaml:"title,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Version     string `json:"version,omitempty" yaml:"version,omitempty"`
}

type SwaggerServer struct {
	URL         string `json:"url,omitempty" yaml:"url,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// ....................path.......method
type SwaggerPath = map[string]map[string]*SwaggerRoute

type SwaggerParam struct {
	Name            string         `json:"name,omitempty" yaml:"name,omitempty"`
	In              string         `json:"in,omitempty" yaml:"in,omitempty"`
	Description     string         `json:"description,omitempty" yaml:"description,omitempty"`
	Type            string         `json:"-" yaml:"-"`
	Schema          map[string]any `json:"schema,omitempty" yaml:"schema,omitempty"`
	Required        bool           `json:"required,omitempty" yaml:"required,omitempty"`
	Deprecated      bool           `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
	AllowEmptyValue bool           `json:"allowEmptyValue,omitempty" yaml:"allowEmptyValue,omitempty"`
}

type SwaggerDesc struct {
	Summary       string `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description   string `json:"description,omitempty" yaml:"description,omitempty"`
	Value         any    `json:"value,omitempty" yaml:"value,omitempty"`
	ExternalValue string `json:"externalValue,omitempty" yaml:"externalValue,omitempty"`
}

type SwaggerRequestBodyContent struct {
	Example  string                  `json:"example,omitempty" yaml:"example,omitempty"`
	Examples map[string]*SwaggerDesc `json:"examples,omitempty" yaml:"examples,omitempty"`
}

type SwaggerRequestBody struct {
	Description string                                `json:"description,omitempty" yaml:"description,omitempty"`
	Content     map[string]*SwaggerRequestBodyContent `json:"content,omitempty" yaml:"value,omitempty"`
	Required    bool                                  `json:"required,omitempty" yaml:"required,omitempty"`
}

type SwaggerRoute struct {
	OperationID string          `json:"operationID,omitempty" yaml:"operationID,omitempty"`
	Summary     string          `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description string          `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        []string        `json:"tags,omitempty" yaml:"tags,omitempty"`
	Parameters  []*SwaggerParam `json:"parameters,omitempty" yaml:"parameters,omitempty"`

	RequestBody *SwaggerRequestBody     `json:"requestBody,omitempty" yaml:"requestBody,omitempty"`
	Responses   map[string]*SwaggerDesc `json:"responses,omitempty" yaml:"responses,omitempty"`
	Examples    map[string]*SwaggerDesc `json:"examples,omitempty" yaml:"examples,omitempty"`
}

func (sr *SwaggerRoute) WithOperationID(v string) *SwaggerRoute {
	sr.OperationID = v
	return sr
}

func (sr *SwaggerRoute) WithSummary(v string) *SwaggerRoute {
	sr.Summary = v
	return sr
}

func (sr *SwaggerRoute) WithDescription(v string) *SwaggerRoute {
	sr.Description = v
	return sr
}

func (sr *SwaggerRoute) WithTags(v ...string) *SwaggerRoute {
	sr.Tags = append(sr.Tags, v...)
	return sr
}

func (sr *SwaggerRoute) WithBody(contentType string, example any) *SwaggerRoute {
	if sr.RequestBody == nil {
		sr.RequestBody = &SwaggerRequestBody{}
	}
	rbc := sr.RequestBody.Content
	if rbc == nil {
		rbc = map[string]*SwaggerRequestBodyContent{}
		sr.RequestBody.Content = rbc
	}
	v := rbc[contentType]
	if v == nil {
		v = &SwaggerRequestBodyContent{}
		rbc[contentType] = v
	}
	switch ex := example.(type) {
	case string:
		v.Example = ex
	default:
		b, err := json.MarshalIndent(ex, "", "\t")
		if err != nil {
			panic(err)
		}
		v.Example = string(b)
	}
	return sr
}

func (sr *SwaggerRoute) WithResponse(name string, ex *SwaggerDesc) *SwaggerRoute {
	if sr.Responses == nil {
		sr.Responses = map[string]*SwaggerDesc{}
	}
	sr.Responses[name] = ex
	return sr
}

func (sr *SwaggerRoute) WithExample(name string, ex *SwaggerDesc) *SwaggerRoute {
	if sr.Examples == nil {
		sr.Examples = map[string]*SwaggerDesc{}
	}
	sr.Examples[name] = ex
	return sr
}

func (sr *SwaggerRoute) WithParams(params []*SwaggerParam) *SwaggerRoute {
	sr.Parameters = append(sr.Parameters, params...)
	return sr
}

func (sr *SwaggerRoute) WithParam(name, desc, in, typ string, required bool, schema map[string]any) *SwaggerRoute {
	p := SwaggerParam{Name: name, Description: desc, In: in, Schema: schema, Required: required}
	if p.In == "" {
		p.In = "path"
	}

	if typ == "" {
		typ = "string"
	}

	if p.Schema == nil {
		p.Schema = map[string]any{}
	}
	p.Schema["type"] = typ

	sr.Parameters = append(sr.Parameters, &p)
	return sr
}

func (r *Router) addRouteInfo(method, path string, desc *SwaggerRoute) *SwaggerRoute {
	p := r.swagger.Paths
	if p == nil {
		p = SwaggerPath{}
		r.swagger.Paths = p
	}

	m := p[path]
	if m == nil {
		m = map[string]*SwaggerRoute{}
		p[path] = m
	}

	if desc == nil {
		desc = &SwaggerRoute{}
	}
	m[strings.ToLower(method)] = desc
	return desc
}

func (r *Router) Swagger() *Swagger {
	return &r.swagger
}

/*
type AutoGenerated struct {
	Openapi    string     `json:"openapi" yaml:"openapi"`
	Info       Info       `json:"info"`
	Servers    []Servers  `json:"servers"`
	Paths      Paths      `json:"paths"`
	Components Components `json:"components"`
}
type License struct {
	Name string `json:"name"`
}
type Info struct {
	Version string  `json:"version"`
	Title   string  `json:"title"`
	License License `json:"license"`
}
type Servers struct {
	URL string `json:"url"`
}
type Schema struct {
	Type   string `json:"type"`
	Format string `json:"format"`
}
type Parameters struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Schema      Schema `json:"schema"`
}
type Schema struct {
	Type string `json:"type"`
}
type XNext struct {
	Description string `json:"description"`
	Schema      Schema `json:"schema"`
}
type Headers struct {
	XNext XNext `json:"x-next"`
}
type Schema struct {
	Ref string `json:"$ref"`
}
type ApplicationJSON struct {
	Schema Schema `json:"schema"`
}
type Content struct {
	ApplicationJSON ApplicationJSON `json:"application/json"`
}
type Num200 struct {
	Description string  `json:"description"`
	Headers     Headers `json:"headers"`
	Content     Content `json:"content"`
}
type Default struct {
	Description string  `json:"description"`
	Content     Content `json:"content"`
}
type Responses struct {
	Num200  Num200  `json:"200"`
	Default Default `json:"default"`
}
type Get struct {
	Summary     string       `json:"summary"`
	OperationID string       `json:"operationId"`
	Tags        []string     `json:"tags"`
	Parameters  []Parameters `json:"parameters"`
	Responses   Responses    `json:"responses"`
}
type Num201 struct {
	Description string `json:"description"`
}
type Responses struct {
	Num201  Num201  `json:"201"`
	Default Default `json:"default"`
}
type Post struct {
	Summary     string    `json:"summary"`
	OperationID string    `json:"operationId"`
	Tags        []string  `json:"tags"`
	Responses   Responses `json:"responses"`
}
type Pets struct {
	Get  Get  `json:"get"`
	Post Post `json:"post"`
}
type PetsPetID struct {
	Get Get `json:"get"`
}
type Paths struct {
	Pets      Pets      `json:"/pets"`
	PetsPetID PetsPetID `json:"/pets/{petId}"`
}
type ID struct {
	Type   string `json:"type"`
	Format string `json:"format"`
}
type Name struct {
	Type string `json:"type"`
}
type Tag struct {
	Type string `json:"type"`
}
type Properties struct {
	ID   ID   `json:"id"`
	Name Name `json:"name"`
	Tag  Tag  `json:"tag"`
}
type Pet struct {
	Type       string     `json:"type"`
	Required   []string   `json:"required"`
	Properties Properties `json:"properties"`
}
type Items struct {
	Ref string `json:"$ref"`
}
type Pets struct {
	Type  string `json:"type"`
	Items Items  `json:"items"`
}
type Code struct {
	Type   string `json:"type"`
	Format string `json:"format"`
}
type Message struct {
	Type string `json:"type"`
}
type Properties struct {
	Code    Code    `json:"code"`
	Message Message `json:"message"`
}
type Error struct {
	Type       string     `json:"type"`
	Required   []string   `json:"required"`
	Properties Properties `json:"properties"`
}
type Schemas struct {
	Pet   Pet   `json:"Pet"`
	Pets  Pets  `json:"Pets"`
	Error Error `json:"Error"`
}
type Components struct {
	Schemas Schemas `json:"schemas"`
}
{
	"openapi": "3.0.0",
	"info": {
		"version": "1.0.0",
		"title": "Swagger Petstore",
		"license": {
			"name": "MIT"
		}
	},
	"servers": [
		{
			"url": "http://petstore.swagger.io/v1"
		}
	],
	"paths": {
		"/pets": {
			"get": {
				"summary": "List all pets",
				"operationId": "listPets",
				"tags": [
					"pets"
				],
				"parameters": [
					{
						"name": "limit",
						"in": "query",
						"description": "How many items to return at one time (max 100)",
						"required": false,
						"schema": {
							"type": "integer",
							"format": "int32"
						}
					}
				],
				"responses": {
					"200": {
						"description": "A paged array of pets",
						"headers": {
							"x-next": {
								"description": "A link to the next page of responses",
								"schema": {
									"type": "string"
								}
							}
						},
						"content": {
							"application/json": {
								"schema": {
									"$ref": "#/components/schemas/Pets"
								}
							}
						}
					},
					"default": {
						"description": "unexpected error",
						"content": {
							"application/json": {
								"schema": {
									"$ref": "#/components/schemas/Error"
								}
							}
						}
					}
				}
			},
			"post": {
				"summary": "Create a pet",
				"operationId": "createPets",
				"tags": [
					"pets"
				],
				"responses": {
					"201": {
						"description": "Null response"
					},
					"default": {
						"description": "unexpected error",
						"content": {
							"application/json": {
								"schema": {
									"$ref": "#/components/schemas/Error"
								}
							}
						}
					}
				}
			}
		},
		"/pets/{petId}": {
			"get": {
				"summary": "Info for a specific pet",
				"operationId": "showPetById",
				"tags": [
					"pets"
				],
				"parameters": [
					{
						"name": "petId",
						"in": "path",
						"required": true,
						"description": "The id of the pet to retrieve",
						"schema": {
							"type": "string"
						}
					}
				],
				"responses": {
					"200": {
						"description": "Expected response to a valid request",
						"content": {
							"application/json": {
								"schema": {
									"$ref": "#/components/schemas/Pet"
								}
							}
						}
					},
					"default": {
						"description": "unexpected error",
						"content": {
							"application/json": {
								"schema": {
									"$ref": "#/components/schemas/Error"
								}
							}
						}
					}
				}
			}
		}
	},
	"components": {
		"schemas": {
			"Pet": {
				"type": "object",
				"required": [
					"id",
					"name"
				],
				"properties": {
					"id": {
						"type": "integer",
						"format": "int64"
					},
					"name": {
						"type": "string"
					},
					"tag": {
						"type": "string"
					}
				}
			},
			"Pets": {
				"type": "array",
				"items": {
					"$ref": "#/components/schemas/Pet"
				}
			},
			"Error": {
				"type": "object",
				"required": [
					"code",
					"message"
				],
				"properties": {
					"code": {
						"type": "integer",
						"format": "int32"
					},
					"message": {
						"type": "string"
					}
				}
			}
		}
	}
}
*/
