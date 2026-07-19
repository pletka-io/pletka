package authpages

import (
	"fmt"
	"html/template"

	"github.com/pletka-io/pletka/pkg/auth"
)

func loginHTML(redirectURL string, registrationEnabled bool) template.HTML {
	registerLink := ""
	if registrationEnabled {
		registerLink = `
            <p class="mt-2 text-center text-sm text-gray-600">
                Need an account?
                <a href="/register" class="font-medium text-pletka-primary hover:text-pletka-secondary">Create one</a>
            </p>`
	}

	return template.HTML(fmt.Sprintf(`
<div class="min-h-[70vh] flex items-center justify-center py-12 px-4 sm:px-6 lg:px-8">
    <div class="max-w-md w-full space-y-8">
        <div>
            <div class="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-pletka-primary">
                <span class="text-2xl text-white">P</span>
            </div>
            <h1 class="mt-6 text-center text-3xl font-bold text-gray-900">Sign in</h1>
            %s
        </div>
        <form id="login-form" class="mt-8 space-y-6" onsubmit="return false;">
            <div class="rounded-md shadow-sm -space-y-px">
                <div>
                    <label for="login" class="sr-only">Email or username</label>
                    <input id="login" name="login" type="text" required
                           class="appearance-none rounded-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-t-md focus:outline-none focus:ring-pletka-primary focus:border-pletka-primary focus:z-10 sm:text-sm"
                           placeholder="Email or username">
                </div>
                <div>
                    <label for="password" class="sr-only">Password</label>
                    <input id="password" name="password" type="password" required
                           class="appearance-none rounded-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-b-md focus:outline-none focus:ring-pletka-primary focus:border-pletka-primary focus:z-10 sm:text-sm"
                           placeholder="Password">
                </div>
            </div>
            <button type="submit"
                    class="group relative w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-pletka-primary hover:bg-pletka-secondary focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-pletka-primary">
                Sign in
            </button>
            <div id="login-result" class="mt-4"></div>
        </form>
    </div>
</div>
<script>
(function () {
  const redirectURL = %s;
  const form = document.getElementById('login-form');
  const result = document.getElementById('login-result');

  function show(tone, title) {
    result.textContent = title;
    result.className = tone === 'success'
      ? 'mt-4 rounded-md bg-green-50 p-4 text-sm font-medium text-green-800'
      : 'mt-4 rounded-md bg-red-50 p-4 text-sm font-medium text-red-800';
  }

  form.addEventListener('submit', function (evt) {
    evt.preventDefault();
    const button = form.querySelector('button[type="submit"]');
    const original = button.textContent;
    button.disabled = true;
    button.textContent = 'Signing in...';

    fetch('/api/v1/auth/login', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        login: form.login.value,
        password: form.password.value
      })
    })
      .then(async function (response) {
        const body = await response.json().catch(function () { return {}; });
        return {ok: response.ok, body: body};
      })
      .then(function (res) {
        if (res.ok) {
          const who = res.body.display_name || res.body.email || 'user';
          show('success', 'Signed in as ' + who + '.');
          setTimeout(function () { window.location.href = redirectURL; }, 500);
          return;
        }
        show('error', res.body.error || 'Invalid credentials');
      })
      .catch(function () {
        show('error', 'Network error. Please try again.');
      })
      .finally(function () {
        button.disabled = false;
        button.textContent = original;
      });
  });
})();
</script>`, registerLink, jsString(redirectURL)))
}

// ssoLoginHTML renders the SSO-only sign-in card: one button to the IdP,
// no password form, no register link.
func ssoLoginHTML(ssoURL string) template.HTML {
	return template.HTML(fmt.Sprintf(`
<div class="min-h-[70vh] flex items-center justify-center py-12 px-4 sm:px-6 lg:px-8">
    <div class="max-w-md w-full space-y-8">
        <div>
            <div class="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-pletka-primary">
                <span class="text-2xl text-white">P</span>
            </div>
            <h1 class="mt-6 text-center text-3xl font-bold text-gray-900">Sign in</h1>
        </div>
        <a href="%s"
           class="group relative w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-pletka-primary hover:bg-pletka-secondary focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-pletka-primary">
            Sign in with SSO
        </a>
    </div>
</div>`, template.HTMLEscapeString(ssoURL)))
}

func registerHTML() template.HTML {
	return template.HTML(`
<div class="min-h-[70vh] flex items-center justify-center py-12 px-4 sm:px-6 lg:px-8">
    <div class="max-w-md w-full space-y-8">
        <div>
            <div class="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-pletka-primary">
                <span class="text-2xl text-white">P</span>
            </div>
            <h1 class="mt-6 text-center text-3xl font-bold text-gray-900">Create account</h1>
            <p class="mt-2 text-center text-sm text-gray-600">
                Already registered?
                <a href="/login" class="font-medium text-pletka-primary hover:text-pletka-secondary">Sign in</a>
            </p>
        </div>
        <form id="register-form" class="mt-8 space-y-4" onsubmit="return false;">
            <div>
                <label for="name" class="block text-sm font-medium text-gray-700">Name</label>
                <input id="name" name="name" type="text" required class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-pletka-primary focus:ring-pletka-primary sm:text-sm">
            </div>
            <div>
                <label for="email" class="block text-sm font-medium text-gray-700">Email</label>
                <input id="email" name="email" type="email" required class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-pletka-primary focus:ring-pletka-primary sm:text-sm">
            </div>
            <div>
                <label for="username" class="block text-sm font-medium text-gray-700">Username</label>
                <input id="username" name="username" type="text" required class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-pletka-primary focus:ring-pletka-primary sm:text-sm">
            </div>
            <div>
                <label for="password" class="block text-sm font-medium text-gray-700">Password</label>
                <input id="password" name="password" type="password" required minlength="12" class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-pletka-primary focus:ring-pletka-primary sm:text-sm">
                <p class="mt-1 text-xs text-gray-500">Use at least 12 characters.</p>
            </div>
            <button type="submit" class="w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-pletka-primary hover:bg-pletka-secondary focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-pletka-primary">
                Create account
            </button>
            <div id="register-result" class="mt-4"></div>
        </form>
    </div>
</div>
<script>
(function () {
  const form = document.getElementById('register-form');
  const result = document.getElementById('register-result');

  function show(tone, title, details) {
    result.textContent = title + (details && details.length ? ' ' + details.join(' ') : '');
    result.className = tone === 'success'
      ? 'mt-4 rounded-md bg-green-50 p-4 text-sm font-medium text-green-800'
      : 'mt-4 rounded-md bg-red-50 p-4 text-sm font-medium text-red-800';
  }

  form.addEventListener('submit', function (evt) {
    evt.preventDefault();
    fetch('/api/v1/auth/register', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        name: form.name.value,
        email: form.email.value,
        username: form.username.value,
        password: form.password.value
      })
    })
      .then(async function (response) {
        const body = await response.json().catch(function () { return {}; });
        return {ok: response.ok, status: response.status, body: body};
      })
      .then(function (res) {
        if (res.ok) {
          const who = res.body.display_name || res.body.username || 'user';
          show('success', 'Welcome, ' + who + '. Redirecting...');
          setTimeout(function () { window.location.href = '/profile'; }, 700);
          return;
        }
        const details = [];
        if (res.body.errors && typeof res.body.errors === 'object') {
          Object.keys(res.body.errors).forEach(function (field) {
            const msg = Array.isArray(res.body.errors[field])
              ? res.body.errors[field].join(', ')
              : String(res.body.errors[field]);
            details.push(field + ': ' + msg);
          });
        }
        show('error', res.body.error || 'Registration failed', details);
      })
      .catch(function () {
        show('error', 'Network error. Please try again.');
      });
  });
})();
</script>`)
}

func profileHTML(principal *auth.Principal) template.HTML {
	display := template.HTMLEscapeString(principal.DisplayName)
	if display == "" {
		display = template.HTMLEscapeString(principal.Slug)
	}
	return template.HTML(fmt.Sprintf(`
<div class="max-w-3xl mx-auto py-10">
    <div class="bg-white shadow-sm rounded-lg border border-gray-200 overflow-hidden">
        <div class="px-6 py-5 border-b border-gray-200">
            <h1 class="text-2xl font-bold text-gray-900">Profile</h1>
            <p class="mt-1 text-sm text-gray-500">Signed in through the weave auth session.</p>
        </div>
        <dl class="divide-y divide-gray-100">
            <div class="px-6 py-4 grid grid-cols-3 gap-4">
                <dt class="text-sm font-medium text-gray-500">Name</dt>
                <dd class="col-span-2 text-sm text-gray-900">%s</dd>
            </div>
            <div class="px-6 py-4 grid grid-cols-3 gap-4">
                <dt class="text-sm font-medium text-gray-500">Username</dt>
                <dd class="col-span-2 text-sm text-gray-900">%s</dd>
            </div>
            <div class="px-6 py-4 grid grid-cols-3 gap-4">
                <dt class="text-sm font-medium text-gray-500">Email</dt>
                <dd class="col-span-2 text-sm text-gray-900">%s</dd>
            </div>
        </dl>
    </div>
</div>`,
		display,
		template.HTMLEscapeString(principal.Slug),
		template.HTMLEscapeString(principal.Email),
	))
}
