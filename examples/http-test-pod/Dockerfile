FROM docker.io/fedora:40

RUN dnf install -y nginx \
    && dnf clean all \
    && rm -rf /var/cache/dnf

COPY nginx.conf /etc/nginx/nginx.conf
COPY entrypoint.sh /entrypoint.sh
COPY html/healthz /var/www/html/healthz
COPY html/index.html /var/www/html/index.html
COPY html/message /var/www/html/message
COPY html/api/items /var/www/html/api/items
COPY html/api/echo /var/www/html/api/echo

RUN mkdir -p /tmp/nginx/{client_body,proxy,fastcgi,uwsgi,scgi} /var/www/html/api \
    && chmod +x /entrypoint.sh \
    && chmod -R a+rwX /tmp/nginx /var/www/html /var/log/nginx /var/lib/nginx

USER 65534:0
EXPOSE 8080
ENTRYPOINT ["/entrypoint.sh"]
