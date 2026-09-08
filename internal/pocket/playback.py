"""Ordered paragraph playback with cancellation and error propagation."""
import queue
import subprocess
import threading


class Playback:
    def __init__(self, gap_ms):
        self.gap = gap_ms / 1000
        self.queue = queue.Queue()
        self.stop = threading.Event()
        self.error = None
        self.thread = threading.Thread(target=self._run)
        self.thread.start()

    def add(self, path):
        self.check()
        self.queue.put(str(path))

    def check(self):
        if self.error is not None:
            raise RuntimeError('speaker playback failed: ' + str(self.error))

    def _run(self):
        try:
            first = True
            while not self.stop.is_set():
                path = self.queue.get()
                if path is None or self.stop.is_set():
                    return
                if not first and self.stop.wait(self.gap):
                    return
                first = False
                process = subprocess.Popen(['/usr/bin/afplay', path])
                try:
                    while process.poll() is None:
                        if self.stop.wait(0.05):
                            return
                    if process.returncode:
                        raise RuntimeError('afplay exited ' + str(process.returncode))
                finally:
                    if process.poll() is None:
                        process.terminate()
                        try:
                            process.wait(timeout=3)
                        except subprocess.TimeoutExpired:
                            process.kill()
                            process.wait()
        except Exception as exc:
            self.error = exc

    def finish(self):
        self.queue.put(None)
        self.thread.join()
        self.check()

    def close(self):
        self.stop.set()
        self.queue.put(None)
        self.thread.join()
