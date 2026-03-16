from locust import HttpUser, task, between, FastHttpUser
import random

# class ProductUser(HttpUser):
#     wait_time = between(1, 2)

#     @task(3)
#     def get_products(self):
#         self.client.get("/products")

#     @task(1)
#     def create_product(self):
#         self.client.post("/products", json={
#             "productId": str(random.randint(1000,9999)),
#             "name": "item",
#             "price": 10.5
#         })

class ProductUser(FastHttpUser):
    # wait_time = between(0.1, 0.2)
    
    @task
    def search(self):
        self.client.get("/products/search?q=Electronics")