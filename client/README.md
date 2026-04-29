This is a [Next.js](https://nextjs.org) project bootstrapped with [`create-next-app`](https://nextjs.org/docs/app/api-reference/cli/create-next-app).

## Getting Started

First, run the development server:

```bash
npm run dev
# or
yarn dev
# or
pnpm dev
# or
bun dev
```

Open [http://localhost:3000](http://localhost:3000) with your browser to see the result.

You can start editing the page by modifying `app/page.tsx`. The page auto-updates as you edit the file.

This project uses [`next/font`](https://nextjs.org/docs/app/building-your-application/optimizing/fonts) to automatically optimize and load [Geist](https://vercel.com/font), a new font family for Vercel.

## Learn More

To learn more about Next.js, take a look at the following resources:

- [Next.js Documentation](https://nextjs.org/docs) - learn about Next.js features and API.
- [Learn Next.js](https://nextjs.org/learn) - an interactive Next.js tutorial.

You can check out [the Next.js GitHub repository](https://github.com/vercel/next.js) - your feedback and contributions are welcome!

## Deploy on Vercel

Vercel preview deployments run the frontend only. The app proxies GraphQL requests to the backend configured by `HYPERINDEX_URL` or `NEXT_PUBLIC_HYPERINDEX_URL`.

If a preview branch targets a shared backend such as `dev.api.hi.gainforest.app`, redeploying the Vercel preview does not rebuild the backend GraphQL schema. After registering or uploading lexicons, restart/redeploy the backend indexer that the preview points to before expecting new typed GraphQL query fields to appear.

See the [Next.js deployment documentation](https://nextjs.org/docs/app/building-your-application/deploying) for general Vercel deployment details.
