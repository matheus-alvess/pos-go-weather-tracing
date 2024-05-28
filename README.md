# Aplicação de Códigos Postais

Esta é uma aplicação simples que recebe um código postal (CEP) e encaminha a solicitação para um serviço de clima. Ele demonstra o uso do OpenTelemetry para rastreamento de solicitações HTTP entre serviços.

## Como testar

Certifique-se de ter o Docker e o Docker Compose instalados na sua máquina.

1. Clone este repositório:

2. Crie e execute os contêineres Docker usando o Docker Compose:

```bash
docker-compose up --build -d
```

3. Após os contêineres serem iniciados, você pode acessar a aplicação no navegador ou através do Postman:

    - Acesse a interface do Zipkin em [http://localhost:9411/zipkin/](http://localhost:9411/zipkin/) para visualizar os traces de rastreamento.
    - Use o Postman ou outro cliente HTTP para enviar uma solicitação POST para [http://localhost:8080/weather](http://localhost:8080/weather) com um corpo JSON contendo o código postal:

   ```json
   {
       "cep": "08410210"
   }
   ```
Isso enviará uma solicitação ao serviço e retornará a resposta do serviço de clima. Os rastreamentos de solicitações podem ser visualizados na interface do Zipkin.


Para desligar os contêineres Docker, execute:
```bash
docker-compose down
```