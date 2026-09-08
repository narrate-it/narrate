import threading
import unittest
from unittest.mock import patch
from playback import Playback

class PlaybackTest(unittest.TestCase):
    def test_ordered_playback(self):
        order = []
        class Process:
            returncode = 0
            def __init__(self,args): order.append(args[-1])
            def poll(self): return 0
        with patch('playback.subprocess.Popen',side_effect=Process):
            player = Playback(0)
            try:
                player.add('first.wav')
                player.add('second.wav')
                player.finish()
            finally: player.close()
        self.assertEqual(order,['first.wav','second.wav'])

    def test_cancel_terminates_active_player(self):
        started = threading.Event()
        stopped = threading.Event()
        class Process:
            returncode = None
            def __init__(self,args): started.set()
            def poll(self): return self.returncode
            def terminate(self): self.returncode = -15; stopped.set()
            def wait(self,timeout=None): return self.returncode
        with patch('playback.subprocess.Popen',side_effect=Process):
            player = Playback(0)
            player.add('first.wav')
            self.assertTrue(started.wait(3))
            player.close()
            self.assertTrue(stopped.is_set())
            self.assertFalse(player.thread.is_alive())

    def test_player_failure_reaches_caller(self):
        class Process:
            returncode = 1
            def poll(self): return 1
        with patch('playback.subprocess.Popen',return_value=Process()):
            player = Playback(0)
            try:
                player.add('broken.wav')
                with self.assertRaisesRegex(RuntimeError,'afplay exited 1'): player.finish()
            finally: player.close()

if __name__ == '__main__': unittest.main()
