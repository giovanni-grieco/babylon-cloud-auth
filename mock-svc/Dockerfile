FROM python:alpine

EXPOSE 8888

WORKDIR /app

COPY requirements.txt /app/requirements.txt

RUN pip install -r requirements.txt

COPY main.py /app/main.py

ENTRYPOINT [ "python3", "main.py" ]