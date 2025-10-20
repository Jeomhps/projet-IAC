Vagrant.configure("2") do |config|
  config.vm.box = "hashicorp-education/ubuntu-24-04"
  config.vm.box_version = "0.1.0"
  config.vm.hostname = "VM-ressources"
  config.vm.network "private_network", ip: "192.168.56.20"

  config.vm.provider "virtualbox" do |vb|
    vb.memory = 4096
    vb.cpus = 2
  end

  # Provisioning Docker + Ansible + 10 containers
  config.vm.provision "shell", inline: <<-SHELL
    set -e

    # Mise à jour + outils
    apt-get update
    apt-get install -y apt-transport-https ca-certificates curl gnupg lsb-release git python3 python3-pip sshpass

    # Docker
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
    echo "deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu focal stable" | sudo tee /etc/apt/sources.list.d/docker.list
    apt-get update
    apt-get install -y docker-ce docker-ce-cli containerd.io
    usermod -aG docker vagrant

    # Ansible
    pip3 install ansible

    # Création du dossier Ansible
    mkdir -p /home/vagrant/ansible/roles/{user-management,docker-resources,audit-logging}
    mkdir -p /home/vagrant/ansible/group_vars

    # Inventory simple
    cat > /home/vagrant/ansible/inventory <<EOF
[local]
127.0.0.1 ansible_connection=local

[all:vars]
ansible_python_interpreter=/usr/bin/python3
EOF

    # site.yml vide mais prêt à remplir
    cat > /home/vagrant/ansible/site.yml <<EOF
- hosts: local
  become: yes
  roles:
    - user-management
    - docker-resources
    - audit-logging
EOF
  SHELL
end
