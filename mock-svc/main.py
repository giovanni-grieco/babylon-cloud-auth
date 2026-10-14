from flask import Flask
import os

app = Flask(__name__)

service_identifier = os.getenv("ID", "x")

@app.route("/")
def hello_world():
    return f"<p>Hello There! This is a mock service #{service_identifier}.</p>"

if __name__ == "__main__":
    app.run(host="0.0.0.0", port="8888", debug=True)