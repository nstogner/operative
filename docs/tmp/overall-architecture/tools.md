# Tools

* LLMs should be presented with a single tool: `run_ipython_cell` which returns the result of running a cell of code in the sandbox.
* Every session that gets created should have a sandbox associated with it.
* Every sandbox should launch a docker container
* This docker container should be lazy-launched upon the first call to `run_ipython_cell` and be named after the session ID
* The session manager should be responsible for keeping track of all active sessions. It should do this using a `index.json` file in the sessions directory.
* Each sandbox container should launch with a python program that runs an IPython kernel.
* This python progress should expose a HTTP endpoint that allows the Go program to run cells of code in the sandbox.
* The HTTP endpoint should return the result of running the cell of code.
* A new package should be introduced that defines the interface for launching sandboxes.
* A docker package should be introduced for implementing local docker sandboxes.
* The runner package should be responsible for running the cells by calling the sandbox manager in RunStep.

NOTE: I have colima running on my local machine.
NOTE: See the code in ./reference/sandboxai which contains some code that can launch docker containers and run an IPython kernel in them. Use this code as inspiration.