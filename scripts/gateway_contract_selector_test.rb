# frozen_string_literal: true
require "minitest/autorun"
require "open3"
require "tmpdir"
require "fileutils"
require "json"
class GatewayContractSelectorTest < Minitest::Test
  ROOT = File.expand_path("..", __dir__)
  def test_selected_model_digest_and_unknown_model_refusal
    %w[general kcmvp].each do |model|
      path = model == "general" ? "testdata/contracts/gateway-api.json" : "testdata/contracts/models/kcmvp/gateway-api.json"
      stdout, stderr, status = Open3.capture3("ruby", "scripts/gateway-contract.rb", model, chdir: ROOT)
      assert status.success?, stderr
      assert_equal JSON.parse(File.read(File.join(ROOT,path))).fetch("source").fetch("swagger_sha256"), stdout.strip
    end
    stdout, _, status = Open3.capture3("ruby", "scripts/gateway-contract.rb", "typo", chdir: ROOT)
    refute status.success?
    assert_empty stdout
    _, _, status = Open3.capture3("make", "-n", "build", "PRODUCT_MODEL=general kcmvp", chdir: ROOT)
    refute status.success?
  end
  def test_explicit_profile_is_validated_and_stamped
    Dir.mktmpdir do |tmp|
      source = JSON.parse(File.read(File.join(ROOT, "testdata/contracts/gateway-api.json")))
      source["source"]["repository"] = "https://example.com/gateway"
      path = File.join(tmp, "gateway-api.json")
      File.write(path, JSON.generate(source))
      out, err, status = Open3.capture3("ruby", "scripts/gateway-contract.rb", "general", path, chdir: ROOT)
      assert status.success?, err
      assert_equal source.dig("source", "swagger_sha256"), out.strip
      source["source"]["repository"] = "https://user:secret@example.com/gateway"
      File.write(path, JSON.generate(source))
      out, _, status = Open3.capture3("ruby", "scripts/gateway-contract.rb", "general", path, chdir: ROOT)
      refute status.success?
      assert_empty out
    end
  end
  def test_wrong_model_contract_cannot_be_stamped
    Dir.mktmpdir do |tmp|
      FileUtils.mkdir_p(File.join(tmp,"scripts"))
      FileUtils.mkdir_p(File.join(tmp,"testdata/contracts"))
      FileUtils.cp(File.join(ROOT,"scripts/gateway-contract.rb"), File.join(tmp,"scripts"))
      source=JSON.parse(File.read(File.join(ROOT,"testdata/contracts/gateway-api.json")))
      source["model"]="kcmvp"
      File.write(File.join(tmp,"testdata/contracts/gateway-api.json"),JSON.generate(source))
      stdout, _, status = Open3.capture3("ruby", File.join(tmp,"scripts/gateway-contract.rb"),"general")
      refute status.success?
      assert_empty stdout
    end
  end
end

class GatewayImmutableImportTest < Minitest::Test
  def git(repo,*args)
    out,err,status=Open3.capture3({"PATH"=>"/usr/bin:/bin","HOME"=>"/nonexistent","GIT_CONFIG_NOSYSTEM"=>"1","GIT_CONFIG_GLOBAL"=>"/dev/null"},
      "/usr/bin/git","-C",repo,*args,unsetenv_others:true)
    assert status.success?,err
    out.strip
  end
  def test_import_ignores_replace_refs_and_ambient_git_poisoning
    require "digest"
    Dir.mktmpdir do |tmp|
      producer=File.join(tmp,"producer");FileUtils.mkdir_p(File.join(producer,"api"));git(producer,"init","-q")
      git(producer,"remote","add","origin","https://github.com/loxilb-io/loxilb-inference-gateway")
      %w[swagger.yml swagger-extras.yml].each{|name|File.write(File.join(producer,"api",name),"original #{name}\n")}
      git(producer,"add",".");git(producer,"-c","user.name=Fixture","-c","user.email=fixture@example.invalid","commit","-qm","original")
      original=git(producer,"rev-parse","HEAD")
      %w[swagger.yml swagger-extras.yml].each{|name|File.write(File.join(producer,"api",name),"replacement #{name}\n")}
      git(producer,"add",".");git(producer,"-c","user.name=Fixture","-c","user.email=fixture@example.invalid","commit","-qm","replacement")
      replacement=git(producer,"rev-parse","HEAD");git(producer,"replace",original,replacement)
      client=File.join(tmp,"client");FileUtils.mkdir_p(File.join(client,"scripts"));FileUtils.mkdir_p(File.join(client,"testdata/contracts"))
      FileUtils.cp(File.join(GatewayContractSelectorTest::ROOT,"scripts/update-gateway-contract.rb"),File.join(client,"scripts"))
      FileUtils.cp(File.join(GatewayContractSelectorTest::ROOT,"testdata/contracts/gateway-api.json"),File.join(client,"testdata/contracts"))
      _,err,status=Open3.capture3({"GIT_DIR"=>"/nonexistent/poison","GIT_WORK_TREE"=>"/nonexistent/poison","GIT_CONFIG_COUNT"=>"1","GIT_CONFIG_KEY_0"=>"remote.origin.url","GIT_CONFIG_VALUE_0"=>"https://wrong.invalid"},
        "ruby",File.join(client,"scripts/update-gateway-contract.rb"),producer,original)
      assert status.success?,err
      actual=JSON.parse(File.read(File.join(client,"testdata/contracts/gateway-api.json"))).fetch("source")
      assert_equal original,actual.fetch("revision")
      assert_equal Digest::SHA256.hexdigest("original swagger.yml\n"),actual.fetch("swagger_sha256")
      assert_equal Digest::SHA256.hexdigest("original swagger-extras.yml\n"),actual.fetch("swagger_extras_sha256")
    end
  end
end
