Vagrant.configure("2") do |config|
  config.vm.box = "hashicorp-education/ubuntu-24-04"
  config.vm.box_version = "0.1.0"
  config.vm.hostname = "VM-ressources"
  config.vm.network "private_network", ip: "192.168.56.20"

  config.vm.provider "virtualbox" do |vb|
    vb.memory = 4096
    vb.cpus = 2
  end

  config.ssh.insert_key = true

  config.vm.provision "ansible" do |ansible|
    ansible.playbook = "install-dependencies.yml"
    ansible.verbose = "v"
    ansible.limit = "all"
  end

end
