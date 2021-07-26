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
