# ibukiRPG 本地推理：llama.cpp（固定版本）+ 一个很薄的 JNI 层（ibuki_llama.cpp）。
# llama.cpp 源码在配置时按 tag 下载并校验 SHA256；离线构建可用 -DIBUKI_LLAMA_SOURCE_DIR=/path/to/llama.cpp 指定本地源码。
cmake_minimum_required(VERSION 3.22.1)
project(ibuki_llama LANGUAGES C CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

set(IBUKI_LLAMA_TAG "v0.5.0" CACHE STRING "llama.cpp release tag")
set(IBUKI_LLAMA_SHA256 "fef9ed754f4e031fb5c663c29260feda4ebc241abb68d64a81c0f1df5f1748e2" CACHE STRING "sha256 of the tag tarball")
set(IBUKI_LLAMA_SOURCE_DIR "" CACHE PATH "use a local llama.cpp checkout instead of downloading")

# llama.cpp 构建选项：只要库本身；CPU 后端按 ABI 编译多个变体，运行时按设备特性（dotprod / i8mm / sve …）选择。
set(BUILD_SHARED_LIBS ON CACHE BOOL "" FORCE)
set(LLAMA_BUILD_COMMON OFF CACHE BOOL "" FORCE)
set(LLAMA_BUILD_TESTS OFF CACHE BOOL "" FORCE)
set(LLAMA_BUILD_TOOLS OFF CACHE BOOL "" FORCE)
set(LLAMA_BUILD_EXAMPLES OFF CACHE BOOL "" FORCE)
set(LLAMA_BUILD_SERVER OFF CACHE BOOL "" FORCE)
set(LLAMA_BUILD_APP OFF CACHE BOOL "" FORCE)
set(LLAMA_CURL OFF CACHE BOOL "" FORCE)
set(LLAMA_OPENSSL OFF CACHE BOOL "" FORCE)
set(GGML_NATIVE OFF CACHE BOOL "" FORCE)
set(GGML_BACKEND_DL ON CACHE BOOL "" FORCE)
set(GGML_CPU_ALL_VARIANTS ON CACHE BOOL "" FORCE)
set(GGML_LLAMAFILE OFF CACHE BOOL "" FORCE)
set(GGML_OPENMP OFF CACHE BOOL "" FORCE)
set(GGML_CPU_KLEIDIAI OFF CACHE BOOL "" FORCE)
set(GGML_VULKAN OFF CACHE BOOL "" FORCE)
set(GGML_OPENCL OFF CACHE BOOL "" FORCE)

if(IBUKI_LLAMA_SOURCE_DIR)
    set(LLAMA_SRC "${IBUKI_LLAMA_SOURCE_DIR}")
else()
    include(FetchContent)
    # 下载目录放在构建目录之外（android/.llama-cache），CI 可以缓存它。
    if(NOT FETCHCONTENT_BASE_DIR)
        set(FETCHCONTENT_BASE_DIR "${CMAKE_CURRENT_LIST_DIR}/../../../../.llama-cache/${IBUKI_LLAMA_TAG}")
    endif()
    FetchContent_Declare(llama_cpp
        URL "https://github.com/ggml-org/llama.cpp/archive/refs/tags/${IBUKI_LLAMA_TAG}.tar.gz"
        URL_HASH SHA256=${IBUKI_LLAMA_SHA256}
        DOWNLOAD_EXTRACT_TIMESTAMP TRUE)
    FetchContent_GetProperties(llama_cpp)
    if(NOT llama_cpp_POPULATED)
        FetchContent_Populate(llama_cpp)
    endif()
    set(LLAMA_SRC "${llama_cpp_SOURCE_DIR}")
endif()

# llama.cpp 会把所有库输出到 <build>/bin；AGP 只打包 CMAKE_LIBRARY_OUTPUT_DIRECTORY 里的 .so，
# 所以配置完成后把 llama / ggml / ggml-cpu-* 的输出目录改回 AGP 的目录。
set(IBUKI_LIB_OUT "${CMAKE_LIBRARY_OUTPUT_DIRECTORY}")
add_subdirectory("${LLAMA_SRC}" llama-build)

function(ibuki_collect_targets dir out)
    get_property(targets DIRECTORY "${dir}" PROPERTY BUILDSYSTEM_TARGETS)
    get_property(subdirs DIRECTORY "${dir}" PROPERTY SUBDIRECTORIES)
    foreach(sub IN LISTS subdirs)
        ibuki_collect_targets("${sub}" sub_targets)
        list(APPEND targets ${sub_targets})
    endforeach()
    set(${out} ${targets} PARENT_SCOPE)
endfunction()

if(IBUKI_LIB_OUT)
    ibuki_collect_targets("${LLAMA_SRC}" llama_targets)
    foreach(t IN LISTS llama_targets)
        get_target_property(t_type ${t} TYPE)
        if(t_type STREQUAL "SHARED_LIBRARY" OR t_type STREQUAL "MODULE_LIBRARY")
            set_target_properties(${t} PROPERTIES LIBRARY_OUTPUT_DIRECTORY "${IBUKI_LIB_OUT}")
            target_link_options(${t} PRIVATE "-Wl,-z,max-page-size=16384")
        endif()
    endforeach()
endif()

add_library(ibuki_llama SHARED ibuki_llama.cpp)
target_link_libraries(ibuki_llama PRIVATE llama ggml android log)
# 16 KB 页面对齐（Android 15+ 设备要求）
target_link_options(ibuki_llama PRIVATE "-Wl,-z,max-page-size=16384")
