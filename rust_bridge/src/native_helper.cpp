#include <windows.h>
#include <cstdint>
#include <functional>

struct SharedMessage {
    void* object;
    void* control;
};

struct Location {
    const char* function;
    const char* file;
    uint32_t line;
    uint32_t pad;
    void* caller;
};

static void release_shared(SharedMessage& message) {
    if (!message.control) return;
    auto control = static_cast<unsigned char*>(message.control);
    if (_InterlockedDecrement(reinterpret_cast<volatile LONG*>(control + 8)) == 0) {
        using VirtualFunction = void(__fastcall*)(void*);
        auto table = *reinterpret_cast<VirtualFunction**>(control);
        table[0](control);
        if (_InterlockedDecrement(reinterpret_cast<volatile LONG*>(control + 12)) == 0) {
            table = *reinterpret_cast<VirtualFunction**>(control);
            table[1](control);
        }
    }
    message = {};
}

extern "C" DWORD schedule_wechat_task(
    uintptr_t core_base,
    uintptr_t app_func_rva,
    uintptr_t scheduler_func_rva,
    uintptr_t schedule_func_rva,
    void (*callback)(void*),
    void* user_data
) {
    Location location{"wxbot.rust", "rust_bridge", 1, 0, nullptr};
    SharedMessage scheduler{}, future{};
    DWORD result = 0;

    __try {
        using App = void* (__fastcall*)();
        using Scheduler = SharedMessage* (__fastcall*)(void*, SharedMessage*);
        using Schedule = SharedMessage* (__fastcall*)(void*, SharedMessage*, Location*, std::function<void()>*, uint32_t);

        void* app = reinterpret_cast<App>(core_base + app_func_rva)();
        if (!app) return 1; // Not ready

        reinterpret_cast<Scheduler>(core_base + scheduler_func_rva)(app, &scheduler);
        if (!scheduler.object || !scheduler.control) return 2;

        std::function<void()> task = [callback, user_data]() {
            __try {
                if (callback) callback(user_data);
            } __except (EXCEPTION_EXECUTE_HANDLER) {
            }
        };

        reinterpret_cast<Schedule>(core_base + schedule_func_rva)(
            scheduler.object,
            &future,
            &location,
            &task,
            1
        );

        release_shared(future);
        release_shared(scheduler);
    } __except (EXCEPTION_EXECUTE_HANDLER) {
        return 3; // Exception
    }

    return result;
}
