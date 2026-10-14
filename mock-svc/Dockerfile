FROM python:alpine

EXPOSE 8888

WORKDIR /app

COPY requirements.txt /app/requirements.txt

COPY main.py /app/main.py

RUN pip install -r requirements.txt

ENTRYPOINT [ "python3", "main.py" ]