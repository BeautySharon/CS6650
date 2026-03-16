# from locust import HttpUser, task, between
# import random

# class MyUser(HttpUser):
#     wait_time = between(0.1, 0.5)

#     @task(3)
#     def do_get(self):
#         self.client.get("/albums")

#     @task(1)
#     def do_post(self):
#         payload = {
#             "title": f"locust-{random.randint(1, 1_000_000)}"
#         }
#         self.client.post("/albums", json=payload)
from locust import task, between
from locust.contrib.fasthttp import FastHttpUser
import random

class MyUser(FastHttpUser):
    wait_time = between(0.1, 0.5)

    @task(3)
    def do_get(self):
        self.client.get("/albums")

    @task(1)
    def do_post(self):
        payload = {
            "title": f"locust-{random.randint(1, 1_000_000)}"
        }
        self.client.post("/albums", json=payload)
