from typing import Dict, Optional
import io
import subprocess
from contextlib import redirect_stdout, redirect_stderr

from fastapi import FastAPI, HTTPException
from IPython.core.interactiveshell import InteractiveShell
from pydantic import BaseModel, Field

# --- Models (merged from sandboxai.api.v1) ---

class RunIPythonCellRequest(BaseModel):
    code: str = Field(..., description="The code to run in the IPython kernel.")
    split_output: Optional[bool] = Field(
        False,
        description="Set to true to split the output into stdout and stderr.",
    )

class RunIPythonCellResult(BaseModel):
    output: Optional[str] = Field(None, description="The stdout and stderr from the IPython kernel interleaved.")
    stdout: Optional[str] = Field(None, description="The stdout from the IPython kernel.")
    stderr: Optional[str] = Field(None, description="The stderr from the IPython kernel.")

# --- Server Implementation ---

app = FastAPI(
    title="Box Daemon",
    version="1.0",
    description="The server that runs python code in a SandboxAI environment.",
)

# Initialize IPython shell
ipy = InteractiveShell.instance()

@app.get("/healthz")
async def healthz():
    return {"status": "OK"}

@app.post(
    "/tools:run_ipython_cell",
    response_model=RunIPythonCellResult,
    summary="Invoke a cell in a stateful IPython (Jupyter) kernel",
)
async def run_ipython_cell(request: RunIPythonCellRequest):
    try:
        if request.split_output:
            stdout_buf = io.StringIO()
            stderr_buf = io.StringIO()

            with redirect_stdout(stdout_buf), redirect_stderr(stderr_buf):
                ipy.run_cell(request.code)

            return RunIPythonCellResult(
                stdout=stdout_buf.getvalue(), stderr=stderr_buf.getvalue()
            )
        else:
            output_buf = io.StringIO()
            with redirect_stdout(output_buf), redirect_stderr(output_buf):
                ipy.run_cell(request.code)

            return RunIPythonCellResult(output=output_buf.getvalue())

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
