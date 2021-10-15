### The Flux-Kube Unified Interface for Task submission

This project provides a unified interface via Flux for 
submission of HPC and K8s tasks which facilitates the use 
of converged environments. This project is based on 
Flux-framework and handles common K8s tasks along with 
traditional HPC requests.

The project has the following software dependencies:
**Software**              | **version**  
----------                | ----------   
Python                    | >= 3.6       
OpenShift Python module   | >= 0.11.2    

To install the additional Python packages, execute: 
```
pip3 install -r requirements.txt
```

#### Building the Unified Interface

To install the interface, follow the typical Autotools-based process, 
specifying the Python installation binary location in the PYTHON 
environment variable if using a virtual environment: 
```
./autogen.sh
PYTHON=/path/to/python3 ./configure
make
make install
```

#### Using the Unified Interface
The interface provides a convenient way for users to start and stop orchestrated (i.e., OpenShift) tasks.  The interface allows users to submit High Performance Computing (HPC) and orchestrated tasks through Flux, reducing the barrier to entry to managing converged workflows.  The interface can manage site-specific OpenShift resources called templates, which are maintained by OpenShift administrators.  In the future, the interface will be extended to create and delete base Kubernetes [Custom Resource Definitions](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/) (CRDs).

Currently, the interface supports two modes of operation: submission of OpenShift tasks through the Flux job manager, which allows them to be created via a typical Flux job submission with a jobspec and direct creation in OpenShift.  The first enables more automated management in that the tasks will be deleted upon reaching the specified walltime, and the second allows creation of long-lived tasks independent of the HPC environment.  You must be logged in to an OpenShift cluster to use the interface.

##### `flux kube`

`flux kube` is a subcommand of flux and features several subcommands of its own:
```
get prints templates and their attributes and
    values.

    supported arguments:
        template_names: names of available OpenShift templates
        template_params: OpenShift template parameters
        templates: OpenShift templates
        also pods, services, etc.

    example:
        flux kube get template_names


submit translates a Flux jobspec V1
    into the corresponding K8s/OpenShift template
    and applies template overrides. Then it
    creates the corresponding objects via `oc create`.

    supported arguments:
        -j FILE, --jobspec=FILE                         Flux jobspec file
        -s JOBSPEC_STRING, --string=JOBSPEC_STRING      Flux jobspec string

    example:
        flux kube submit -j kubejob.yaml


cancel translates a Flux jobspec V1
    into the corresponding K8s/OpenShift template
    and applies template overrides. Then it
    deletes the corresponding objects via `oc delete`.

    supported arguments:
        -j FILE, --jobspec=FILE                         Flux jobspec file
        -s JOBSPEC_STRING, --string=JOBSPEC_STRING      Flux jobspec string

    example:
        flux kube submit -j kubejob.yaml


mini takes a template name and
    overrides as arguments and constructs a skeleton
    jobspec based on the arguments. It then invokes
    submit to create the objects or cancel
    to delete them.

    supported arguments:
        mini 
            supported arguments:
                submit|cancel
                    supported arguments: 
                        TEMPLATE_NAME
                        -o OVERRIDE_KV, --overrides=OVERRIDE_KV 

    example:
        flux kube mini submit nginx-template -o '{"APPLICATION_DOMAIN": "nginxtest-user.apps.domain.com"}'
```

##### Submitting OpenShift jobs to Flux with `flux job submit`
Creating an OpenShift job in Flux via the Unified Interface functions exactly the same as a normal job from the user perspective. The Unified Interface makes used of a jobtap plugin that bypasses the typical allocation and execution system. The bypass is necessary to ensure that Flux does not attempt to allocate resources it does not manage or execute the command corresponding to the OpenShift template. The jobtap module reads the jobspec and sends it to the `flux-kube` daemon which creates the objects corresponding to the selected template with the specified template overrides.  Job cancellation functions as usual as well.

The following is an example jobspec file that will create an nginx service based on the OpenShift template:

```yaml
version: 1
resources:
  - type: slot
    count: 1
    label: default
    with:
      - type: core
        count: 1
tasks:
  - command: [ "nginx-template" ]
    slot: default
    count:
      per_slot: 1
attributes:
  system:
    duration: 120
    cwd: "/home/flux"
    flux-kube: 1
    exec:
      test: {}
    template_overrides:
      APPLICATION_DOMAIN: nginxtest-user.apps.domain.com
```

Here is an example of submitting the nginx job to Flux (system names changed):

```console
$ flux kube get pod
No resources found in user namespace.

$ flux kube get service
No resources found in user namespace.

$ flux kube get route
No resources found in user namespace.

$ flux job submit test-nginx.json 
fy9HPZ1H
$ flux jobs -a
       JOBID USER     NAME       ST NTASKS NNODES  RUNTIME NODELIST
    fy9HPZ1H milroy1  nginx-test  R      1      1   1.604s kubernetes
     fXKJKDy milroy1  flux        R      1      1   2.148m node1
$ flux kube get route
NAME       HOST/PORT                                   PATH   SERVICES   PORT    TERMINATION          WILDCARD
nginx      nginxtest-user.apps.domain.com                      nginx    <all>    reencrypt/Redirect   None

$ flux kube get pod  
NAME                READY   STATUS      RESTARTS   AGE
nginx-1-2g5sm    1/1     Running     0          14s
nginx-1-deploy   0/1     Completed   0          18s
```
Check to ensure the nginx default page can be accessed:
```console
$ curl -k nginxtest-user.apps.domain.com
<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
<style>
    body {
        width: 35em;
        margin: 0 auto;
        font-family: Tahoma, Verdana, Arial, sans-serif;
    }
</style>
</head>
<body>
<h1>Welcome to nginx!</h1>
<p>If you see this page, the nginx web server is successfully installed and
working. Further configuration is required.</p>

<p>For online documentation and support please refer to
<a href="http://nginx.org/">nginx.org</a>.<br/>
Commercial support is available at
<a href="http://nginx.com/">nginx.com</a>.</p>

<p><em>Thank you for using nginx.</em></p>
</body>
</html>
```

When the execution time (120s, see jobspec) is reached, all OpenShift objects are deleted:
```console
$ flux jobs -a
       JOBID USER     NAME       ST NTASKS NNODES  RUNTIME NODELIST
     fXKJKDy milroy1  flux        R      1      1   5.354m node1
    fy9HPZ1H milroy1  nginx-test CD      1      1       2m kubernetes

$ flux kube get pod
No resources found in user namespace.

$ flux kube get service
No resources found in user namespace.

$ flux kube get route  
No resources found in user namespace.
```