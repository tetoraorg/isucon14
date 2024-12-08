# isucon-ansible

## 必要ツール

- Ansible: [Installation Guide](https://docs.ansible.com/ansible/latest/installation_guide/index.html)
- Python 3
- Cloned repository

## ガイド

### 初動セットアップ

`ansible/group_vars/all.yml`にサーバー共通設定を書く

```yaml
app_name: isuapp
unit_name: isuapp.go.service
github_token: github_pat_XXXXX
```

`s[0-9]*/host_vars.yml`に各サーバーの設定を書く

```yaml
ansible_host: 127.0.0.1
ansible_user: isucon
ansible_password: password
```

Ansibleを実行する\
実行元のレポジトリが各サーバーにクローンされる

```bash
cd ansible
ansible-playbook playbooks/setup.yml # --limit s1,s2
```

### デプロイ

```bash
cd ansible
ansible-playbook playbooks/deploy.yml # --limit s1,s2 --extra-vars "branch=main"
```

## ユーティリティ

`~/.bashrc`を参照
