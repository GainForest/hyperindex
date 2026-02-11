#!/usr/bin/env node

/**
 * Generate an ES256 JWK private key for ATProto OAuth confidential client authentication.
 *
 * Usage: node scripts/generate-jwk.js
 *
 * Copy the output and add it to your .env file as ATPROTO_JWK_PRIVATE
 */

const crypto = require('crypto')

async function generateJWK() {
  // Generate an EC key pair using P-256 curve (required for ES256)
  const { privateKey } = await crypto.subtle.generateKey(
    {
      name: 'ECDSA',
      namedCurve: 'P-256',
    },
    true, // extractable
    ['sign', 'verify']
  )

  // Export as JWK
  const jwk = await crypto.subtle.exportKey('jwk', privateKey)

  // Add key ID and algorithm
  jwk.kid = `key-${Date.now()}`
  jwk.alg = 'ES256'
  jwk.use = 'sig'

  console.log('\n=== ES256 Private Key (JWK) ===\n')
  console.log('Add this to your .env file as ATPROTO_JWK_PRIVATE:\n')
  console.log(JSON.stringify(jwk))
  console.log('\n')
}

generateJWK().catch(console.error)
