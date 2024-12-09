#!/bin/bash -eux

export PROD=$([[ "${1:-}" == "prod" ]] && echo true || echo false)

cd $REPOSITORY_DIR/go
go mod download
go build -o $APP_NAME

cd $REPOSITORY_DIR

sudo cp ./common/nginx.conf /etc/nginx/nginx.conf
sudo cp ./common/$APP_NAME.conf /etc/nginx/sites-available/$APP_NAME.conf
sudo cp ./common/mysqld.cnf /etc/mysql/mysql.conf.d/mysqld.cnf
sudo cp ./common/sysctl.conf /etc/sysctl.conf
sudo cp ./$SERVER_NAME/$UNIT_NAME /etc/systemd/system/$UNIT_NAME
sudo cp ./$SERVER_NAME/env.sh /home/isucon/env.sh

# Log
# NOTE: mysql-slow.log must be readable by both mysql and isucon user
sudo chmod +r /var/log/*
sudo sudo usermod -aG mysql isucon
sudo rm -rf /var/log/mysql/mysql-slow.log &&
  sudo touch /var/log/mysql/mysql-slow.log &&
  sudo chmod +r /var/log/mysql/mysql-slow.log &&
  sudo chown mysql:mysql /var/log/mysql &&
  sudo chown mysql:mysql /var/log/mysql/mysql-slow.log
sudo rm -rf /var/log/nginx/access.log &&
  sudo touch /var/log/nginx/access.log &&
  sudo chmod +r /var/log/nginx/access.log

sudo systemctl daemon-reload
sudo systemctl restart $UNIT_NAME
sudo systemctl restart nginx
sudo systemctl stop mysql
sudo systemctl disable mysql
sudo systemctl stop isuride-matcher.service
sudo systemctl disable isuride-matcher.service
sudo sysctl -p
# Slow Query Log
if $PROD; then
  sudo mysql -e 'SET GLOBAL slow_query_log = OFF;'
else
  sudo mysql -e 'SET GLOBAL long_query_time = 0; SET GLOBAL slow_query_log = ON; SET GLOBAL slow_query_log_file = "/var/log/mysql/mysql-slow.log";'
fi
