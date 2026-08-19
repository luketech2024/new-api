#!/bin/bash
# ============================================================
# MySQL 初始化脚本 (仅首次创建数据卷时执行)
# new_api 数据库和 newapi 用户由 MySQL 镜像自动创建
# 此脚本创建 wechat_epay 数据库及其用户
# ============================================================

mysql -uroot -p"$MYSQL_ROOT_PASSWORD" <<EOSQL
CREATE DATABASE IF NOT EXISTS \`$WECHAT_DB_NAME\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS '$WECHAT_DB_USER'@'%' IDENTIFIED BY '$WECHAT_DB_PASSWORD';
GRANT ALL PRIVILEGES ON \`$WECHAT_DB_NAME\`.* TO '$WECHAT_DB_USER'@'%';
FLUSH PRIVILEGES;
EOSQL

echo "==> Database '$WECHAT_DB_NAME' and user '$WECHAT_DB_USER' created."
