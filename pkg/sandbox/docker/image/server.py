import asyncio
import io
import queue
import sys
import threading
import uuid
import time
import logging
from concurrent import futures
from contextlib import redirect_stdout, redirect_stderr
from typing import Optional
import traceback

import grpc
from IPython.core.interactiveshell import InteractiveShell

import sandbox_pb2
import sandbox_pb2_grpc

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')
logger = logging.getLogger("sandbox")

class SandboxServicer(sandbox_pb2_grpc.SandboxServicer):
    def __init__(self):
        self.ipy = InteractiveShell.instance()
        self.response_queue = queue.Queue()
        self.pending_prompts = {} # id -> threading.Event + result placeholder
        self.lock = threading.Lock()
        
        # Inject custom functions into IPython namespace
        self.ipy.user_ns["prompt_model"] = self.prompt_model
        self.ipy.user_ns["prompt_self"] = self.prompt_self

    def prompt_model(self, prompt: str) -> str:
        req_id = str(uuid.uuid4())
        event = threading.Event()
        
        with self.lock:
            self.pending_prompts[req_id] = {"event": event, "response": None}
            
        logger.info(f"Prompting model: {req_id}")
        self.response_queue.put(sandbox_pb2.ServerMessage(
            prompt_model=sandbox_pb2.PromptModelRequest(prompt=prompt, id=req_id)
        ))
        
        # Wait for response
        event.wait()
        
        with self.lock:
            data = self.pending_prompts.pop(req_id)
            return data["response"]

    def prompt_self(self, message: str):
        logger.info("Prompting self")
        self.response_queue.put(sandbox_pb2.ServerMessage(
            prompt_self=sandbox_pb2.PromptSelfRequest(message=message)
        ))

    def RunStream(self, request_iterator, context):
        # Create a new queue for this stream
        q = queue.Queue()
        with self.lock:
            self.response_queue = q

        # Start a thread to consume requests
        consumer_thread = threading.Thread(target=self._consume_requests, args=(request_iterator, context))
        consumer_thread.daemon = True
        consumer_thread.start()
        
        # Main loop yields responses from queue
        while True:
            try:
                msg = q.get(timeout=1.0)
                # logger.info(f"Yielding message: {msg.WhichOneof('payload')}")
                yield msg
            except queue.Empty:
                if not context.is_active():
                    logger.info("Context inactive, stopping RunStream")
                    break
                continue
            except Exception as e:
                logger.error(f"Error yielding response: {e}")
                break



    def _consume_requests(self, request_iterator, context):
        try:
            for req in request_iterator:
                if req.HasField("run_cell"):
                    # Run in a separate thread to allow prompt_responses to be processed
                    t = threading.Thread(target=self._handle_run_cell, args=(req.run_cell,))
                    t.start()
                elif req.HasField("prompt_model_response"):
                    self._handle_prompt_response(req.prompt_model_response)
                elif req.HasField("cancel"):
                     # Handle cancellation if needed (e.g. interrupt kernel)
                     pass
        except Exception as e:
            logger.error(f"Error consuming requests: {e}")
            traceback.print_exc()

    def _handle_prompt_response(self, resp: sandbox_pb2.PromptModelResponse):
        logger.info(f"Received prompt response: {resp.id}")
        with self.lock:
            if resp.id in self.pending_prompts:
                self.pending_prompts[resp.id]["response"] = resp.response
                self.pending_prompts[resp.id]["event"].set()

    class OutputStream(io.TextIOBase):
        def __init__(self, queue, is_stderr=False):
            self.queue = queue
            self.is_stderr = is_stderr

        def write(self, s):
            if s:
                self.queue.put(sandbox_pb2.ServerMessage(
                    output=sandbox_pb2.Output(text=s, is_stderr=self.is_stderr)
                ))
            return len(s)
            
        def flush(self):
            pass

    def _handle_run_cell(self, req: sandbox_pb2.RunCellRequest):
        logger.info("Running cell")
        # Setup redirection
        stdout_stream = self.OutputStream(self.response_queue, is_stderr=False)
        stderr_stream = self.OutputStream(self.response_queue, is_stderr=True)
        
        # We need to capture the output buffers properly for the final result as well, 
        # or just rely on streaming.
        # The legacy interface expects a final result with accumulated output.
        # But we can also just stream everything and return empty/full logic in Result.
        
        # In this implementation, we stream everything AND capture it for the result?
        # Or just let the client accumulate. 
        # The `Result` object in proto has output/stdout/stderr. 
        # Let's accumulate locally to send the final summary.
        
        captured_stdout = io.StringIO()
        captured_stderr = io.StringIO()
        
        class TeeStream(io.TextIOBase):
            def __init__(self, stream1, stream2):
                self.stream1 = stream1
                self.stream2 = stream2
            def write(self, s):
                self.stream1.write(s)
                self.stream2.write(s)
                return len(s)
            def flush(self):
                self.stream1.flush()
                self.stream2.flush()

        tee_stdout = TeeStream(stdout_stream, captured_stdout)
        tee_stderr = TeeStream(stderr_stream, captured_stderr)

        with redirect_stdout(tee_stdout), redirect_stderr(tee_stderr):
            try:
                self.ipy.run_cell(req.code)
            except Exception as e:
                # Should be caught by ipython usually, but just in case
                tee_stderr.write(str(e))

        # Send result
        self.response_queue.put(sandbox_pb2.ServerMessage(
            run_cell_result=sandbox_pb2.RunCellResult(
                output=captured_stdout.getvalue() + captured_stderr.getvalue(),
                stdout=captured_stdout.getvalue(),
                stderr=captured_stderr.getvalue(),
                success=True # TODO check execution success properly
            )
        ))

def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    sandbox_pb2_grpc.add_SandboxServicer_to_server(SandboxServicer(), server)
    server.add_insecure_port('[::]:8000')
    logger.info("Starting sandbox server on :8000")
    server.start()
    server.wait_for_termination()

if __name__ == '__main__':
    serve()
