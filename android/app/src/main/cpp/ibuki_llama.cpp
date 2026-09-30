// ibukiRPG 的 llama.cpp JNI 层：加载 GGUF、套用模型自带的聊天模板、流式生成、可取消。
// 只使用 llama.h 公共 API；所有调用都由 Kotlin 端放在后台线程，并保证同一模型实例串行使用。
#include <jni.h>
#include <android/log.h>
#include <unistd.h>

#include <algorithm>
#include <atomic>
#include <string>
#include <vector>

#include "ggml-backend.h"
#include "llama.h"

#define TAG "ibuki-llama"
#define LOGI(...) __android_log_print(ANDROID_LOG_INFO, TAG, __VA_ARGS__)
#define LOGW(...) __android_log_print(ANDROID_LOG_WARN, TAG, __VA_ARGS__)
#define LOGE(...) __android_log_print(ANDROID_LOG_ERROR, TAG, __VA_ARGS__)

namespace {

struct Handle {
    llama_model *model = nullptr;
    llama_context *ctx = nullptr;
    const llama_vocab *vocab = nullptr;
    int n_ctx = 0;
    int n_batch = 512;
    std::atomic<bool> cancel{false};
};

std::atomic<bool> g_backend_ready{false};

void log_callback(ggml_log_level level, const char *text, void *) {
    if (level == GGML_LOG_LEVEL_ERROR) {
        __android_log_print(ANDROID_LOG_ERROR, TAG, "%s", text);
    } else if (level == GGML_LOG_LEVEL_WARN) {
        __android_log_print(ANDROID_LOG_WARN, TAG, "%s", text);
    }
}

bool abort_cb(void *data) {
    auto *h = static_cast<Handle *>(data);
    return h->cancel.load();
}

std::string jstr(JNIEnv *env, jstring s) {
    if (!s) return {};
    const char *c = env->GetStringUTFChars(s, nullptr);
    std::string out(c ? c : "");
    if (c) env->ReleaseStringUTFChars(s, c);
    return out;
}

// Java 的 GetStringUTFChars 是“修改版 UTF-8”，中文（BMP）与标准 UTF-8 一致；
// 表情等补充平面字符用 getBytes(UTF_8) 从 Kotlin 端传字节更稳，所以提示词以 byte[] 传入。
std::string jbytes(JNIEnv *env, jbyteArray a) {
    if (!a) return {};
    jsize n = env->GetArrayLength(a);
    std::string out(static_cast<size_t>(n), '\0');
    env->GetByteArrayRegion(a, 0, n, reinterpret_cast<jbyte *>(&out[0]));
    return out;
}

// 返回 buf 中以完整 UTF-8 字符结尾的最长前缀长度（其余字节留到下一个 token 再输出）。
size_t utf8_complete_prefix(const std::string &buf) {
    size_t n = buf.size();
    size_t i = n;
    // 回退最多 3 个续字节，找到最后一个起始字节
    size_t back = 0;
    while (i > 0 && back < 4) {
        unsigned char c = static_cast<unsigned char>(buf[i - 1]);
        if ((c & 0xC0) != 0x80) {
            size_t need = (c & 0x80) == 0 ? 1 : (c & 0xE0) == 0xC0 ? 2 : (c & 0xF0) == 0xE0 ? 3 : (c & 0xF8) == 0xF0 ? 4 : 1;
            size_t have = n - (i - 1);
            return have >= need ? n : i - 1;
        }
        --i;
        ++back;
    }
    return n;
}

std::vector<llama_token> tokenize(const llama_vocab *vocab, const std::string &text, bool add_special) {
    int n = -llama_tokenize(vocab, text.data(), static_cast<int32_t>(text.size()), nullptr, 0, add_special, true);
    if (n <= 0) return {};
    std::vector<llama_token> out(static_cast<size_t>(n));
    int got = llama_tokenize(vocab, text.data(), static_cast<int32_t>(text.size()), out.data(), n, add_special, true);
    if (got < 0) return {};
    out.resize(static_cast<size_t>(got));
    return out;
}

}  // namespace

extern "C" {

JNIEXPORT void JNICALL
Java_com_guyu2233_ibukirpg_app_llm_LlamaNative_nativeInit(JNIEnv *env, jclass, jstring lib_dir) {
    if (g_backend_ready.exchange(true)) return;
    llama_log_set(log_callback, nullptr);
    std::string dir = jstr(env, lib_dir);
    // CPU 后端按设备特性选择最合适的变体（libggml-cpu-*.so 位于 nativeLibraryDir）
    ggml_backend_load_all_from_path(dir.c_str());
    llama_backend_init();
    LOGI("llama backend ready (%s)", dir.c_str());
}

JNIEXPORT jstring JNICALL
Java_com_guyu2233_ibukirpg_app_llm_LlamaNative_nativeSystemInfo(JNIEnv *env, jclass) {
    return env->NewStringUTF(llama_print_system_info());
}

JNIEXPORT jlong JNICALL
Java_com_guyu2233_ibukirpg_app_llm_LlamaNative_nativeLoad(JNIEnv *env, jclass, jstring jpath, jint n_ctx, jint n_threads, jint n_gpu_layers) {
    std::string path = jstr(env, jpath);
    llama_model_params mp = llama_model_default_params();
    mp.load_mode = LLAMA_LOAD_MODE_MMAP;  // 权重用 mmap 映射：不占 Java 堆，内存紧张时可被系统回收后按需重读
    mp.n_gpu_layers = n_gpu_layers;  // 当前构建只有 CPU 后端，非 0 时 llama.cpp 会忽略
    llama_model *model = llama_model_load_from_file(path.c_str(), mp);
    if (!model) {
        LOGE("load failed: %s", path.c_str());
        return 0;
    }
    auto *h = new Handle();
    h->model = model;
    h->vocab = llama_model_get_vocab(model);
    llama_context_params cp = llama_context_default_params();
    int train = llama_model_n_ctx_train(model);
    int ctx = std::max(512, static_cast<int>(n_ctx));
    if (train > 0 && ctx > train) ctx = train;
    cp.n_ctx = static_cast<uint32_t>(ctx);
    cp.n_batch = 512;
    cp.n_ubatch = 512;
    int threads = n_threads > 0 ? n_threads : std::max(2, std::min(4, static_cast<int>(sysconf(_SC_NPROCESSORS_ONLN)) - 2));
    cp.n_threads = threads;
    cp.n_threads_batch = threads;
    cp.abort_callback = abort_cb;
    cp.abort_callback_data = h;
    cp.no_perf = true;
    h->ctx = llama_init_from_model(model, cp);
    if (!h->ctx) {
        LOGE("context init failed (n_ctx=%d)", ctx);
        llama_model_free(model);
        delete h;
        return 0;
    }
    h->n_ctx = static_cast<int>(llama_n_ctx(h->ctx));
    LOGI("model loaded: n_ctx=%d threads=%d", h->n_ctx, threads);
    return reinterpret_cast<jlong>(h);
}

JNIEXPORT jint JNICALL
Java_com_guyu2233_ibukirpg_app_llm_LlamaNative_nativeContextSize(JNIEnv *, jclass, jlong handle) {
    auto *h = reinterpret_cast<Handle *>(handle);
    return h ? h->n_ctx : 0;
}

JNIEXPORT void JNICALL
Java_com_guyu2233_ibukirpg_app_llm_LlamaNative_nativeFree(JNIEnv *, jclass, jlong handle) {
    auto *h = reinterpret_cast<Handle *>(handle);
    if (!h) return;
    if (h->ctx) llama_free(h->ctx);
    if (h->model) llama_model_free(h->model);
    delete h;
}

JNIEXPORT void JNICALL
Java_com_guyu2233_ibukirpg_app_llm_LlamaNative_nativeCancel(JNIEnv *, jclass, jlong handle) {
    auto *h = reinterpret_cast<Handle *>(handle);
    if (h) h->cancel.store(true);
}

JNIEXPORT jstring JNICALL
Java_com_guyu2233_ibukirpg_app_llm_LlamaNative_nativeChatTemplate(JNIEnv *env, jclass, jlong handle) {
    auto *h = reinterpret_cast<Handle *>(handle);
    if (!h) return nullptr;
    const char *t = llama_model_chat_template(h->model, nullptr);
    return t ? env->NewStringUTF(t) : nullptr;
}

// 用模型自带（或内置识别的）聊天模板拼提示词；模板不受支持时返回 null，由 Kotlin 端退回通用格式。
JNIEXPORT jbyteArray JNICALL
Java_com_guyu2233_ibukirpg_app_llm_LlamaNative_nativeApplyTemplate(JNIEnv *env, jclass, jlong handle, jobjectArray roles, jobjectArray contents) {
    auto *h = reinterpret_cast<Handle *>(handle);
    if (!h) return nullptr;
    const char *tmpl = llama_model_chat_template(h->model, nullptr);
    if (!tmpl) return nullptr;
    jsize n = env->GetArrayLength(roles);
    std::vector<std::string> r(static_cast<size_t>(n)), c(static_cast<size_t>(n));
    std::vector<llama_chat_message> msgs(static_cast<size_t>(n));
    size_t total = 0;
    for (jsize i = 0; i < n; ++i) {
        auto *jr = static_cast<jstring>(env->GetObjectArrayElement(roles, i));
        auto *jc = static_cast<jbyteArray>(env->GetObjectArrayElement(contents, i));
        r[i] = jstr(env, jr);
        c[i] = jbytes(env, jc);
        env->DeleteLocalRef(jr);
        env->DeleteLocalRef(jc);
        total += r[i].size() + c[i].size();
    }
    for (jsize i = 0; i < n; ++i) msgs[i] = {r[i].c_str(), c[i].c_str()};
    std::vector<char> buf(total * 2 + 1024);
    int32_t len = llama_chat_apply_template(tmpl, msgs.data(), msgs.size(), true, buf.data(), static_cast<int32_t>(buf.size()));
    if (len < 0) return nullptr;
    if (static_cast<size_t>(len) > buf.size()) {
        buf.resize(static_cast<size_t>(len) + 1);
        len = llama_chat_apply_template(tmpl, msgs.data(), msgs.size(), true, buf.data(), static_cast<int32_t>(buf.size()));
        if (len < 0) return nullptr;
    }
    jbyteArray out = env->NewByteArray(len);
    env->SetByteArrayRegion(out, 0, len, reinterpret_cast<const jbyte *>(buf.data()));
    return out;
}

// 流式生成。callback.onBytes(byte[]) 返回 false 表示停止（客户端断开 / 用户取消）。
// 返回值：0 正常结束（EOS 或达到 max_tokens），1 被取消，-1 提示词为空，-2 解码失败。
JNIEXPORT jint JNICALL
Java_com_guyu2233_ibukirpg_app_llm_LlamaNative_nativeGenerate(JNIEnv *env, jclass, jlong handle, jbyteArray jprompt, jint max_tokens,
                                                            jfloat temp, jfloat top_p, jint top_k, jint seed, jobject callback) {
    auto *h = reinterpret_cast<Handle *>(handle);
    if (!h) return -2;
    h->cancel.store(false);
    jclass cbc = env->GetObjectClass(callback);
    jmethodID on_bytes = env->GetMethodID(cbc, "onBytes", "([B)Z");
    if (!on_bytes) return -2;

    std::string prompt = jbytes(env, jprompt);
    // 聊天模板已经带了 BOS 等特殊标记时不要重复添加
    bool add_bos = llama_vocab_get_add_bos(h->vocab);
    std::vector<llama_token> tokens = tokenize(h->vocab, prompt, add_bos);
    if (tokens.empty()) return -1;

    int budget = h->n_ctx - std::max(16, static_cast<int>(max_tokens)) - 4;
    if (budget < 64) budget = std::max(64, h->n_ctx / 2);
    if (static_cast<int>(tokens.size()) > budget) {
        // 超长：保留开头（系统提示）与结尾（最近的内容），丢掉中间
        int head = budget / 3;
        int tail = budget - head;
        std::vector<llama_token> cut(tokens.begin(), tokens.begin() + head);
        cut.insert(cut.end(), tokens.end() - tail, tokens.end());
        LOGW("prompt truncated %zu -> %zu tokens", tokens.size(), cut.size());
        tokens.swap(cut);
    }

    llama_memory_clear(llama_get_memory(h->ctx), true);

    for (size_t i = 0; i < tokens.size(); i += static_cast<size_t>(h->n_batch)) {
        int n = static_cast<int>(std::min(tokens.size() - i, static_cast<size_t>(h->n_batch)));
        int rc = llama_decode(h->ctx, llama_batch_get_one(tokens.data() + i, n));
        if (h->cancel.load() || rc == 2) return 1;
        if (rc != 0) {
            LOGE("prompt decode failed rc=%d", rc);
            return -2;
        }
    }

    llama_sampler *smpl = llama_sampler_chain_init(llama_sampler_chain_default_params());
    llama_sampler_chain_add(smpl, llama_sampler_init_penalties(llama_vocab_n_tokens(h->vocab), 64, 1.1f, 0.0f, 0.0f));
    llama_sampler_chain_add(smpl, llama_sampler_init_top_k(top_k > 0 ? top_k : 40));
    llama_sampler_chain_add(smpl, llama_sampler_init_top_p(top_p, 1));
    if (temp <= 0.0f) {
        llama_sampler_chain_add(smpl, llama_sampler_init_greedy());
    } else {
        llama_sampler_chain_add(smpl, llama_sampler_init_temp(temp));
        llama_sampler_chain_add(smpl, llama_sampler_init_dist(static_cast<uint32_t>(seed)));
    }

    int status = 0;
    std::string pending;
    char piece[256];
    int left = h->n_ctx - static_cast<int>(tokens.size()) - 1;
    int limit = std::min(static_cast<int>(max_tokens), left);
    for (int i = 0; i < limit; ++i) {
        if (h->cancel.load()) { status = 1; break; }
        llama_token tok = llama_sampler_sample(smpl, h->ctx, -1);
        if (llama_vocab_is_eog(h->vocab, tok)) break;
        int n = llama_token_to_piece(h->vocab, tok, piece, sizeof(piece), 0, false);
        if (n > 0) {
            pending.append(piece, static_cast<size_t>(n));
            size_t ok = utf8_complete_prefix(pending);
            if (ok > 0) {
                jbyteArray arr = env->NewByteArray(static_cast<jsize>(ok));
                env->SetByteArrayRegion(arr, 0, static_cast<jsize>(ok), reinterpret_cast<const jbyte *>(pending.data()));
                jboolean go_on = env->CallBooleanMethod(callback, on_bytes, arr);
                env->DeleteLocalRef(arr);
                pending.erase(0, ok);
                if (env->ExceptionCheck() || !go_on) { status = 1; break; }
            }
        }
        llama_token next = tok;
        int rc = llama_decode(h->ctx, llama_batch_get_one(&next, 1));
        if (rc == 2 || h->cancel.load()) { status = 1; break; }
        if (rc != 0) { status = -2; break; }
    }
    llama_sampler_free(smpl);
    return status;
}

}  // extern "C"
