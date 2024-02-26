/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./**/*.{html,js,templ}"],
  theme: {
    extend: {},
  },
  plugins: [require("@tailwindcss/typography"), require("daisyui")],
  daisyui: {
    themes: ["lofi"],
  },
  safelist: [
    {
      // Remove this for production. This includes ALL tailwind classes...
      // For development it made sense to just add this.
      pattern: /.*/
    }
  ]
}

