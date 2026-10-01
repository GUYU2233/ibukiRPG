import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
    id("org.jetbrains.kotlin.plugin.serialization")
}

val appVersionName = "0.2.0-alpha1"

// llama.cpp 原生库的 ABI：默认 arm64-v8a + x86_64（模拟器）；CI 可用 -Pibuki.llama.abis=arm64-v8a 缩短构建时间
val llamaAbis: List<String> = ((project.findProperty("ibuki.llama.abis") as String?) ?: "arm64-v8a,x86_64")
    .split(',').map { it.trim() }.filter { it.isNotEmpty() }

// 发布签名：密钥库放在仓库之外（默认 ~/.ibukirpg/keystore.properties），见 docs/android.md。
val keystorePropsFile = providers.environmentVariable("IBUKIRPG_KEYSTORE_PROPERTIES")
    .orElse(System.getProperty("user.home") + "/.ibukirpg/keystore.properties")
    .map { file(it) }
    .get()
val keystoreProps = Properties().apply {
    if (keystorePropsFile.exists()) keystorePropsFile.inputStream().use { load(it) }
}

// CI：环境变量优先（ANDROID_KEYSTORE_FILE 为解码后的密钥库路径；其余来自 GitHub Actions secrets）。
// 任一缺失（例如来自 fork 的 pull_request）时回退到 keystore.properties；都没有则 release APK 不签名。
fun signingValue(env: String, prop: String): String? =
    System.getenv(env)?.takeIf { it.isNotBlank() } ?: keystoreProps.getProperty(prop)?.takeIf { it.isNotBlank() }

val envSigningComplete = listOf("ANDROID_KEYSTORE_FILE", "ANDROID_KEYSTORE_PASSWORD", "ANDROID_KEY_ALIAS", "ANDROID_KEY_PASSWORD")
    .all { !System.getenv(it).isNullOrBlank() }
val releaseStoreFile: String? =
    if (envSigningComplete) System.getenv("ANDROID_KEYSTORE_FILE") else keystoreProps.getProperty("storeFile")

android {
    namespace = "com.guyu2233.ibukirpg.app"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.guyu2233.ibukirpg"
        minSdk = 24
        targetSdk = 36
        versionCode = 6
        versionName = appVersionName
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        vectorDrawables.useSupportLibrary = true
        // 与 gomobile AAR 中的 libgojni.so 保持一致（否则 x86 设备会因缺少引擎库而崩溃）
        ndk { abiFilters += listOf("armeabi-v7a", "arm64-v8a", "x86_64") }
        // llama.cpp（本地 GGUF 推理）：只为 64 位 ABI 构建；armeabi-v7a 设备上本地模型不可用，其余功能照常
        externalNativeBuild {
            cmake {
                abiFilters += llamaAbis
                arguments += listOf("-DCMAKE_BUILD_TYPE=Release", "-DANDROID_STL=c++_shared")
                (project.findProperty("ibuki.llama.src") as String?)?.let { arguments += "-DIBUKI_LLAMA_SOURCE_DIR=$it" }
            }
        }
    }

    externalNativeBuild {
        cmake {
            path = file("src/main/cpp/CMakeLists.txt")
            version = "3.31.6"
        }
    }
    ndkVersion = "27.3.13750724"

    signingConfigs {
        if (releaseStoreFile != null && file(releaseStoreFile).exists()) {
            create("release") {
                storeFile = file(releaseStoreFile)
                if (envSigningComplete) {
                    storePassword = System.getenv("ANDROID_KEYSTORE_PASSWORD")
                    keyAlias = System.getenv("ANDROID_KEY_ALIAS")
                    keyPassword = System.getenv("ANDROID_KEY_PASSWORD")
                } else {
                    storePassword = signingValue("ANDROID_KEYSTORE_PASSWORD", "storePassword")
                    keyAlias = signingValue("ANDROID_KEY_ALIAS", "keyAlias")
                    keyPassword = signingValue("ANDROID_KEY_PASSWORD", "keyPassword")
                }
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            signingConfig = signingConfigs.findByName("release")
        }
        debug {
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    testOptions {
        unitTests {
            isIncludeAndroidResources = true
            all {
                it.systemProperty("roborazzi.test.record", "true")
                // 可用 -Pibuki.shots.dir=… / -Pibuki.fixtures.dir=… 把截图渲染到别处、改用本地夹具（私有故事包截图）
                it.systemProperty(
                    "ibuki.shots.dir",
                    (project.findProperty("ibuki.shots.dir") as String?) ?: rootProject.file("../build/screenshots").absolutePath,
                )
                (project.findProperty("ibuki.fixtures.dir") as String?)?.let { d -> it.systemProperty("ibuki.fixtures.dir", d) }
                // 夹具目录变化时强制重跑
                it.inputs.property("ibukiFixturesDir", (project.findProperty("ibuki.fixtures.dir") as String?) ?: "")
                it.inputs.property("ibukiShotsDir", (project.findProperty("ibuki.shots.dir") as String?) ?: "")
                it.maxHeapSize = "2g"
            }
        }
    }
    packaging {
        resources.excludes += "/META-INF/{AL2.0,LGPL2.1}"
        // 安装时把原生库解压到 nativeLibraryDir：llama.cpp 要在该目录里扫描 libggml-cpu-*.so，
        // 按设备 CPU 特性（dotprod / i8mm / SVE …）选择最快的变体
        jniLibs.useLegacyPackaging = true
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
    }
}

// 输出文件名：ibukiRPG-v0.2.0-alpha1.apk
androidComponents {
    onVariants { variant ->
        variant.outputs.forEach { output ->
            val suffix = if (variant.buildType == "release") "" else "-${variant.buildType}"
            (output as? com.android.build.api.variant.impl.VariantOutputImpl)
                ?.outputFileName?.set("ibukiRPG-v$appVersionName$suffix.apk")
        }
    }
}

dependencies {
    // gomobile 生成的 Go 引擎（make android-aar）
    implementation(fileTree(mapOf("dir" to "libs", "include" to listOf("*.aar"))))

    val composeBom = platform("androidx.compose:compose-bom:2025.10.01")
    implementation(composeBom)
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.compose.material:material-icons-extended:1.7.8")
    implementation("androidx.activity:activity-compose:1.11.0")
    implementation("androidx.core:core-ktx:1.17.0")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.9.4")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.9.4")
    implementation("androidx.navigation:navigation-compose:2.9.5")
    implementation("androidx.datastore:datastore-preferences:1.1.7")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.9.0")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.10.2")
    debugImplementation("androidx.compose.ui:ui-tooling")

    testImplementation("junit:junit:4.13.2")
    // JVM 截图测试（Robolectric + Roborazzi）：渲染真实 Composable，数据来自真实引擎导出的夹具
    testImplementation("org.robolectric:robolectric:4.16")
    testImplementation("io.github.takahirom.roborazzi:roborazzi:1.76.0")
    testImplementation("io.github.takahirom.roborazzi:roborazzi-compose:1.76.0")
    testImplementation("androidx.compose.ui:ui-test-junit4")
    debugImplementation("androidx.compose.ui:ui-test-manifest")
}
