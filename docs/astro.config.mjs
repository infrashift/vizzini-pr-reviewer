// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// Deployed to GitHub Pages by .github/workflows/docs-release.yml.
// https://astro.build/config
export default defineConfig({
	site: 'https://infrashift.github.io',
	base: '/vizzini-pr-reviewer',
	integrations: [
		starlight({
			title: 'Vizzini',
			description:
				'A pure-Go AI code review engine: token-aware diff chunking, any model through LiteLLM, and every answer validated against a CUE schema.',
			favicon: '/favicon.svg',
			customCss: ['./src/styles/custom.css'],
			social: [
				{
					icon: 'github',
					label: 'GitHub',
					href: 'https://github.com/infrashift/vizzini-pr-reviewer',
				},
			],
			editLink: {
				baseUrl: 'https://github.com/infrashift/vizzini-pr-reviewer/edit/main/docs/',
			},
			sidebar: [
				{
					label: 'Start Here',
					items: [
						{ label: 'Overview', slug: 'docs' },
						{ label: 'Getting Started', slug: 'docs/getting-started' },
					],
				},
				{
					label: 'Guides',
					items: [
						{ label: 'How a Review Works', slug: 'docs/guides/how-it-works' },
						{ label: 'Embedding in Go', slug: 'docs/guides/embedding' },
						{ label: 'Lint-Aware Reviews', slug: 'docs/guides/lint' },
						{ label: 'Dependency Walkthroughs', slug: 'docs/guides/walkthrough' },
						{ label: 'Risk & Auto-Approve', slug: 'docs/guides/risk' },
						{ label: 'Running Offline', slug: 'docs/guides/offline' },
					],
				},
				{
					label: 'Reference',
					items: [
						{ label: 'CLI Reference', slug: 'docs/reference/cli' },
						{ label: 'Review Schema', slug: 'docs/reference/review-schema' },
						{ label: 'Annotated Diff Format', slug: 'docs/reference/diff-format' },
						{ label: 'Make Targets', slug: 'docs/reference/make-targets' },
					],
				},
				{
					label: 'Project',
					collapsed: true,
					items: [
						{ label: 'Roadmap', slug: 'docs/project/roadmap' },
						{ label: 'Design Decisions', slug: 'docs/project/decisions' },
						{ label: 'Contributing', slug: 'docs/project/contributing' },
					],
				},
			],
		}),
	],
});
