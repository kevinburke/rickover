http_archive(
    name = "io_bazel_rules_go",
    url = "https://github.com/bazelbuild/rules_go/releases/download/0.8.1/rules_go-0.8.1.tar.gz",
    sha256 = "90bb270d0a92ed5c83558b2797346917c46547f6f7103e648941ecdb6b9d0e72",
)

load("@io_bazel_rules_go//go:def.bzl", "go_rules_dependencies", "go_register_toolchains", "go_repository")
go_rules_dependencies()
go_register_toolchains()

go_repository(
    name = "com_github_kevinburke_go_uuid",
    importpath = "github.com/kevinburke/go.uuid",
    commit = "24443c65ec63d9e040fd4cedf0f1048b5d3544f7",
)

go_repository(
    name = "com_github_kevinburke_rest",
    importpath = "github.com/kevinburke/rest",
    commit = "5a70172425b1704eedc967b5abf0610866bd26a1",
)

go_repository(
    name = "com_github_mattn_go_colorable",
    importpath = "github.com/mattn/go-colorable",
    commit = "3fa8c76f9daed4067e4a806fb7e4dc86455c6d6a",
)

go_repository(
    name = "com_github_mattn_go_isatty",
    importpath = "github.com/mattn/go-isatty",
    commit = "fc9e8d8ef48496124e79ae0df75490096eccf6fe",
)

go_repository(
    name = "com_github_go_stack_stack",
    importpath = "github.com/go-stack/stack",
    commit = "54be5f394ed2c3e19dac9134a40a95ba5a017f7b",
)

go_repository(
    name = "com_github_inconshreveable_log15",
    importpath = "github.com/inconshreveable/log15",
    commit = "74a0988b5f804e8ce9ff74fca4f16980776dff29",
)
