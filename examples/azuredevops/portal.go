package azuredevops

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog/v2"
	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/examples/azuredevops/agentpool"
	"github.com/verifa/horizon/examples/azuredevops/project"
	"github.com/verifa/horizon/examples/azuredevops/vmss"
	"github.com/verifa/horizon/pkg/gateway"
	"github.com/verifa/horizon/pkg/hz"
)

var Portal = hz.Portal{
	ObjectMeta: hz.ObjectMeta{
		Name:    "azuredevops",
		Account: hz.RootAccount,
	},
	Spec: &hz.PortalSpec{
		DisplayName: "Azure DevOps",
		Icon:        gateway.IconCodeBracketSquare,
	},
}

type PortalHandler struct {
	Conn *nats.Conn
}

func (h *PortalHandler) Router() *chi.Mux {
	r := chi.NewRouter()
	logger := httplog.NewLogger("portal-azuredevops", httplog.Options{
		JSON:             false,
		LogLevel:         slog.LevelInfo,
		Concise:          true,
		RequestHeaders:   true,
		MessageFieldName: "message",
		QuietDownRoutes: []string{
			"/",
			"/ping",
		},
		QuietDownPeriod: 10 * time.Second,
	})
	r.Use(httplog.RequestLogger(logger))
	r.Get("/", h.get)
	r.Post("/projects", h.postProjects)
	r.Get("/projects/new", h.getProjectsNew)
	r.Get("/projects/{project}", h.getProjectByName)

	r.Get("/projects/{project}/vmscalesets/new", h.getProjectVMScaleSetsNew)
	r.Post("/projects/{project}/vmscalesets", h.postProjectVMScaleSets)

	r.Get("/projects/{project}/agentpools/new", h.getProjectAgentPoolNew)
	r.Post("/projects/{project}/agentpools", h.postProjectAgentPool)
	return r
}

func (h *PortalHandler) get(rw http.ResponseWriter, req *http.Request) {
	rendr := PortalRenderer{
		Account: req.Header.Get(hz.RequestAccount),
		Portal:  req.Header.Get(hz.RequestPortal),
	}

	client := hz.ObjectClient[project.Project]{
		Client: hz.NewClient(
			h.Conn,
			hz.WithClientDefaultManager(),
			hz.WithClientSessionFromRequest(req),
		),
	}

	projects, err := client.List(req.Context())
	if err != nil {
		rw.Write([]byte(fmt.Sprintf("error listing projects: %v", err)))
		return
	}

	_ = rendr.home(projects).Render(req.Context(), rw)
}

func (h *PortalHandler) postProjects(
	rw http.ResponseWriter,
	req *http.Request,
) {
	rendr := PortalRenderer{
		Account: req.Header.Get(hz.RequestAccount),
		Portal:  req.Header.Get(hz.RequestPortal),
	}

	if err := req.ParseForm(); err != nil {
		_ = rendr.newProjectForm("", fmt.Errorf("parsing form: %w", err)).
			Render(req.Context(), rw)
		return
	}

	projectName := req.PostForm.Get("project-name")
	newProject := project.Project{
		ObjectMeta: hz.ObjectMeta{
			Account: req.Header.Get(hz.RequestAccount),
			Name:    projectName,
			Finalizers: &hz.Finalizers{
				project.Finalizer,
			},
		},
		Spec: &project.ProjectSpec{},
	}

	client := hz.NewClient(
		h.Conn,
		hz.WithClientSessionFromRequest(req),
		hz.WithClientDefaultManager(),
	)

	if err := client.Create(req.Context(), hz.WithCreateObject(newProject)); err != nil {
		_ = rendr.newProjectForm(projectName, err).
			Render(req.Context(), rw)
		return
	}

	// If all went well, redirect to the project page?
	rw.Header().Add("HX-Redirect", rendr.URL("projects", projectName))
}

func (h *PortalHandler) getProjectsNew(
	rw http.ResponseWriter,
	req *http.Request,
) {
	rendr := PortalRenderer{
		Account: req.Header.Get(hz.RequestAccount),
		Portal:  req.Header.Get(hz.RequestPortal),
	}

	_ = rendr.newProject().Render(req.Context(), rw)
}

func (h *PortalHandler) getProjectByName(
	rw http.ResponseWriter,
	req *http.Request,
) {
	rendr := PortalRenderer{
		Account: req.Header.Get(hz.RequestAccount),
		Portal:  req.Header.Get(hz.RequestPortal),
	}

	projectName := chi.URLParam(req, "project")
	client := hz.NewClient(
		h.Conn,
		hz.WithClientDefaultManager(),
		hz.WithClientSessionFromRequest(req),
	)
	projClient := hz.ObjectClient[project.Project]{
		Client: client,
	}

	proj, err := projClient.Get(req.Context(), hz.WithGetKey(hz.ObjectKey{
		Account: req.Header.Get(hz.RequestAccount),
		Name:    projectName,
	}))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusNotFound)
		return
	}

	if req.Header.Get("HX-Request") == "true" {
		if req.Header.Get("HZ-VM-Scale-Sets") == "true" {
			vmssClient := hz.ObjectClient[vmss.VMScaleSet]{
				Client: client,
			}
			vmScaleSets, err := vmssClient.List(req.Context())
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = rendr.vmScaleSetsTable(vmScaleSets).Render(req.Context(), rw)
			return
		}
		if req.Header.Get("HZ-Agent-Pools") == "true" {

			apClient := hz.ObjectClient[agentpool.AgentPool]{
				Client: client,
			}
			agentPools, err := apClient.List(req.Context())
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = rendr.agentPoolsTable(agentPools).Render(req.Context(), rw)
			return
		}
		http.Error(rw, "unknown HX-Request", http.StatusBadRequest)
	}

	_ = rendr.project(proj).Render(req.Context(), rw)
}

func (h *PortalHandler) getProjectVMScaleSetsNew(
	rw http.ResponseWriter,
	req *http.Request,
) {
	rendr := PortalRenderer{
		Account: req.Header.Get(hz.RequestAccount),
		Portal:  req.Header.Get(hz.RequestPortal),
	}

	projectClient := hz.ObjectClient[project.Project]{
		Client: hz.NewClient(
			h.Conn,
			hz.WithClientDefaultManager(),
			hz.WithClientSessionFromRequest(req),
		),
	}
	adoProject, err := projectClient.Get(
		req.Context(),
		hz.WithGetKey(hz.ObjectKey{
			Account: req.Header.Get(hz.RequestAccount),
			Name:    chi.URLParam(req, "project"),
		}),
	)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusNotFound)
		return
	}

	_ = rendr.newProjectVMScaleSet(adoProject).Render(req.Context(), rw)
}

func (h *PortalHandler) postProjectVMScaleSets(
	rw http.ResponseWriter,
	req *http.Request,
) {
	rendr := PortalRenderer{
		Account: req.Header.Get(hz.RequestAccount),
		Portal:  req.Header.Get(hz.RequestPortal),
	}
	client := hz.NewClient(
		h.Conn,
		hz.WithClientDefaultManager(),
		hz.WithClientSessionFromRequest(req),
	)
	projectClient := hz.ObjectClient[project.Project]{
		Client: client,
	}
	adoProject, err := projectClient.Get(
		req.Context(),
		hz.WithGetKey(hz.ObjectKey{
			Account: req.Header.Get(hz.RequestAccount),
			Name:    chi.URLParam(req, "project"),
		}),
	)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusNotFound)
		return
	}
	if err := req.ParseForm(); err != nil {
		_ = rendr.newProjectVMScaleSetForm(
			adoProject,
			vmss.VMScaleSet{
				Spec: &vmss.VMScaleSetSpec{},
			},
			fmt.Errorf("error parsing form: %w", err),
		).Render(req.Context(), rw)
		return
	}

	scaleSet := vmss.VMScaleSet{
		ObjectMeta: hz.ObjectMeta{
			Account: req.Header.Get(hz.RequestAccount),
			Name:    req.PostForm.Get("name"),
			Finalizers: &hz.Finalizers{
				vmss.Finalizer,
			},
		},
		Spec: &vmss.VMScaleSetSpec{
			Location:          req.PostForm.Get("location"),
			ResourceGroupName: req.PostForm.Get("resource-group-name"),
			VMSize:            req.PostForm.Get("vm-size"),
		},
	}

	if err := client.Create(req.Context(), hz.WithCreateObject(scaleSet)); err != nil {
		_ = rendr.newProjectVMScaleSetForm(
			adoProject,
			scaleSet,
			err,
		).Render(req.Context(), rw)
		return
	}
	// If successful, redirect back to the project page.
	rw.Header().Add("HX-Redirect", rendr.URL("projects", adoProject.Name))
}

func (h *PortalHandler) getProjectAgentPoolNew(
	rw http.ResponseWriter,
	req *http.Request,
) {
	rendr := PortalRenderer{
		Account: req.Header.Get(hz.RequestAccount),
		Portal:  req.Header.Get(hz.RequestPortal),
	}

	projectClient := hz.ObjectClient[project.Project]{
		Client: hz.NewClient(
			h.Conn,
			hz.WithClientDefaultManager(),
			hz.WithClientSessionFromRequest(req),
		),
	}
	adoProject, err := projectClient.Get(
		req.Context(),
		hz.WithGetKey(hz.ObjectKey{
			Account: req.Header.Get(hz.RequestAccount),
			Name:    chi.URLParam(req, "project"),
		}),
	)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusNotFound)
		return
	}
	vmssClient := hz.ObjectClient[vmss.VMScaleSet]{
		Client: hz.NewClient(
			h.Conn,
			hz.WithClientDefaultManager(),
			hz.WithClientSessionFromRequest(req),
		),
	}
	vmScaleSets, err := vmssClient.List(req.Context())
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = rendr.newProjectAgentPool(adoProject, vmScaleSets).
		Render(req.Context(), rw)
}

func (h *PortalHandler) postProjectAgentPool(
	rw http.ResponseWriter,
	req *http.Request,
) {
	rendr := PortalRenderer{
		Account: req.Header.Get(hz.RequestAccount),
		Portal:  req.Header.Get(hz.RequestPortal),
	}
	client := hz.NewClient(
		h.Conn,
		hz.WithClientDefaultManager(),
		hz.WithClientSessionFromRequest(req),
	)
	projectClient := hz.ObjectClient[project.Project]{
		Client: client,
	}
	adoProject, err := projectClient.Get(
		req.Context(),
		hz.WithGetKey(hz.ObjectKey{
			Account: req.Header.Get(hz.RequestAccount),
			Name:    chi.URLParam(req, "project"),
		}),
	)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusNotFound)
		return
	}
	vmssClient := hz.ObjectClient[vmss.VMScaleSet]{
		Client: hz.NewClient(
			h.Conn,
			hz.WithClientDefaultManager(),
			hz.WithClientSessionFromRequest(req),
		),
	}
	vmScaleSets, err := vmssClient.List(req.Context())
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := req.ParseForm(); err != nil {
		_ = rendr.newProjectAgentPoolForm(
			adoProject,
			agentpool.AgentPool{
				Spec: &agentpool.AgentPoolSpec{},
			},
			vmScaleSets,
			fmt.Errorf("error parsing form: %w", err),
		).Render(req.Context(), rw)
		return
	}

	agentPool := agentpool.AgentPool{
		ObjectMeta: hz.ObjectMeta{
			Account: req.Header.Get(hz.RequestAccount),
			Name:    req.PostForm.Get("name"),
			Finalizers: &hz.Finalizers{
				agentpool.Finalizer,
			},
		},
		Spec: &agentpool.AgentPoolSpec{
			VMScaleSetRef: agentpool.VMScaleSetRef{
				Name: req.PostForm.Get("vm-scaleset-ref"),
			},
			ProjectRef: agentpool.ProjectRef{
				Name: chi.URLParam(req, "project"),
			},
		},
	}

	if err := client.Create(req.Context(), hz.WithCreateObject(agentPool)); err != nil {
		_ = rendr.newProjectAgentPoolForm(
			adoProject,
			agentPool,
			vmScaleSets,
			err,
		).Render(req.Context(), rw)
		return
	}
	// If successful, redirect back to the project page.
	rw.WriteHeader(http.StatusOK)
	rw.Header().Add("HX-Redirect", rendr.URL("projects", adoProject.Name))
}

type PortalRenderer struct {
	Account string
	Portal  string
}

func (r *PortalRenderer) URLSafe(steps ...string) templ.SafeURL {
	base := fmt.Sprintf("/accounts/%s/portal/%s", r.Account, r.Portal)
	path := append([]string{base}, steps...)
	return templ.URL(strings.Join(path, "/"))
}

func (r *PortalRenderer) URL(steps ...string) string {
	return string(r.URLSafe(steps...))
}
