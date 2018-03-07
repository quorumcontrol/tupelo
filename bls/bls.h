#include <stdint.h>
#include <stdlib.h>
#include <stdbool.h>

#define ITERATION 4

#define LARGE_ALPHATILDE 2787

#define LARGE_ETILDE 456

#define LARGE_E_END_RANGE 119

#define LARGE_E_START 596

#define LARGE_M2_TILDE 1024

#define LARGE_MASTER_SECRET 256

#define LARGE_MTILDE 593

#define LARGE_MVECT 592

#define LARGE_NONCE 80

#define LARGE_PRIME 1024

#define LARGE_RTILDE 672

#define LARGE_UTILDE 592

#define LARGE_VPRIME 2128

#define LARGE_VPRIME_PRIME 2724

#define LARGE_VPRIME_TILDE 673

#define LARGE_VTILDE 3060

enum ErrorCode {
  Success = 0,
  CommonInvalidParam1 = 100,
  CommonInvalidParam2 = 101,
  CommonInvalidParam3 = 102,
  CommonInvalidParam4 = 103,
  CommonInvalidParam5 = 104,
  CommonInvalidParam6 = 105,
  CommonInvalidParam7 = 106,
  CommonInvalidParam8 = 107,
  CommonInvalidParam9 = 108,
  CommonInvalidParam10 = 109,
  CommonInvalidParam11 = 110,
  CommonInvalidParam12 = 111,
  CommonInvalidState = 112,
  CommonInvalidStructure = 113,
  CommonIOError = 114,
  AnoncredsRevocationAccumulatorIsFull = 115,
  AnoncredsInvalidRevocationAccumulatorIndex = 116,
  AnoncredsClaimRevoked = 117,
  AnoncredsProofRejected = 118,
};
typedef uintptr_t ErrorCode;

typedef ErrorCode (*FFITailTake)(const void*, uint32_t, const void**);

typedef ErrorCode (*FFITailPut)(const void*, const void*);

/*
 * Returns bytes representation of generator point.
 *
 * Note: Returned buffer lifetime is the same as generator instance.
 *
 * # Arguments
 * * `gen` - Generator instance pointer
 * * `bytes_p` - Pointer that will contains bytes buffer
 * * `bytes_len_p` - Pointer that will contains bytes buffer len
 */
ErrorCode indy_crypto_bls_generator_as_bytes(const void *gen,
                                             const uint8_t **bytes_p,
                                             size_t *bytes_len_p);

/*
 * Deallocates generator instance.
 *
 * # Arguments
 * * `gen` - Generator instance pointer
 */
ErrorCode indy_crypto_bls_generator_free(const void *gen);

/*
 * Creates and returns generator point from bytes representation.
 *
 * Note: Generator instance deallocation must be performed by calling indy_crypto_bls_generator_free
 *
 * # Arguments
 * * `bytes` - Bytes buffer pointer
 * * `bytes_len` - Bytes buffer len
 * * `gen_p` - Reference that will contain generator instance pointer
 */
ErrorCode indy_crypto_bls_generator_from_bytes(const uint8_t *bytes,
                                               size_t bytes_len,
                                               const void **gen_p);

/*
 * Creates and returns random generator point that satisfy BLS algorithm requirements.
 *
 * BLS algorithm requires choosing of generator point that must be known to all parties.
 * The most of BLS methods require generator to be provided.
 *
 * Note: Generator instance deallocation must be performed by calling indy_crypto_bls_generator_free
 *
 * # Arguments
 * * `gen_p` - Reference that will contain generator instance pointer
 */
ErrorCode indy_crypto_bls_generator_new(const void **gen_p);

/*
 * Returns bytes representation of multi signature.
 *
 * Note: Returned buffer lifetime is the same as multi signature instance.
 *
 * # Arguments
 * * `multi_sig` - Multi signature instance pointer
 * * `bytes_p` - Pointer that will contains bytes buffer
 * * `bytes_len_p` - Pointer that will contains bytes buffer len
 */
ErrorCode indy_crypto_bls_multi_signature_as_bytes(const void *multi_sig,
                                                   const uint8_t **bytes_p,
                                                   size_t *bytes_len_p);

/*
 * Deallocates multi signature instance.
 *
 * # Arguments
 * * `multi_sig` - Multi signature instance pointer
 */
ErrorCode indy_crypto_bls_multi_signature_free(const void *multi_sig);

/*
 * Creates and returns multi signature from bytes representation.
 *
 * Note: Multi signature instance deallocation must be performed by calling indy_crypto_bls_multi_signature_free
 *
 * # Arguments
 * * `bytes` - Bytes buffer pointer
 * * `bytes_len` - Bytes buffer len
 * * `multi_sig_p` - Reference that will contain multi signature instance pointer
 */
ErrorCode indy_crypto_bls_multi_signature_from_bytes(const uint8_t *bytes,
                                                     size_t bytes_len,
                                                     const void **multi_sig_p);

/*
 * Creates and returns multi signature for provided list of signatures.
 *
 * Note: Multi signature instance deallocation must be performed by calling indy_crypto_bls_multi_signature_free.
 *
 * # Arguments
 * * `signatures` - Signature instance pointers array
 * * `signatures_len` - Signature instance pointers array len
 * * `multi_sig_p` - Reference that will contain multi signature instance pointer
 */
ErrorCode indy_crypto_bls_multi_signature_new(const void *const *signatures,
                                              size_t signatures_len,
                                              const void **multi_sig_p);

/*
 * Signs the message and returns signature.
 *
 * Note: allocated buffer referenced by (signature_p, signature_len_p) must be
 * deallocated by calling indy_crypto_bls_free_array.
 *
 * # Arguments
 *
 * * `message` - Message to sign buffer pointer
 * * `message_len` - Message to sign buffer len
 * * `sign_key` - Pointer to Sign Key instance
 * * `signature_p` - Reference that will contain Signture Instance pointer
 */
ErrorCode indy_crypto_bls_sign(const uint8_t *message,
                               size_t message_len,
                               const void *sign_key,
                               const void **signature_p);

/*
 * Returns bytes representation of sign key.
 *
 * Note: Returned buffer lifetime is the same as sign key instance.
 *
 * # Arguments
 * * `sign_key` - Sign key instance pointer
 * * `bytes_p` - Pointer that will contains bytes buffer
 * * `bytes_len_p` - Pointer that will contains bytes buffer len
 */
ErrorCode indy_crypto_bls_sign_key_as_bytes(const void *sign_key,
                                            const uint8_t **bytes_p,
                                            size_t *bytes_len_p);

/*
 * Deallocates sign key instance.
 *
 * # Arguments
 * * `sign_key` - Sign key instance pointer
 */
ErrorCode indy_crypto_bls_sign_key_free(const void *sign_key);

/*
 * Creates and returns sign key from bytes representation.
 *
 * Note: Sign key instance deallocation must be performed by calling indy_crypto_bls_sign_key_free
 *
 * # Arguments
 * * `bytes` - Bytes buffer pointer
 * * `bytes_len` - Bytes buffer len
 * * `sign_key_p` - Reference that will contain sign key instance pointer
 */
ErrorCode indy_crypto_bls_sign_key_from_bytes(const uint8_t *bytes,
                                              size_t bytes_len,
                                              const void **sign_key_p);

/*
 * Creates and returns random (or seeded from seed) BLS sign key algorithm requirements.
 *
 * Note: Sign Key instance deallocation must be performed by calling indy_crypto_bls_sign_key_free.
 *
 * # Arguments
 * * `seed` - Seed buffer pointer. For random generation null must be passed.
 * * `seed` - Seed buffer len.
 * * `gen_p` - Reference that will contain sign key instance pointer
 */
ErrorCode indy_crypto_bls_sign_key_new(const uint8_t *seed,
                                       size_t seed_len,
                                       const void **sign_key_p);

/*
 * Returns bytes representation of signature.
 *
 * Note: Returned buffer lifetime is the same as signature instance.
 *
 * # Arguments
 * * `signature` - Signature instance pointer
 * * `bytes_p` - Pointer that will contains bytes buffer
 * * `bytes_len_p` - Pointer that will contains bytes buffer len
 */
ErrorCode indy_crypto_bls_signature_as_bytes(const void *signature,
                                             const uint8_t **bytes_p,
                                             size_t *bytes_len_p);

/*
 * Deallocates signature instance.
 *
 * # Arguments
 * * `signature` - Signature instance pointer
 */
ErrorCode indy_crypto_bls_signature_free(const void *signature);

/*
 * Creates and returns signature from bytes representation.
 *
 * Note: Signature instance deallocation must be performed by calling indy_crypto_bls_signature_free
 *
 * # Arguments
 * * `bytes` - Bytes buffer pointer
 * * `bytes_len` - Bytes buffer len
 * * `signature_p` - Reference that will contain signature instance pointer
 */
ErrorCode indy_crypto_bls_signature_from_bytes(const uint8_t *bytes,
                                               size_t bytes_len,
                                               const void **signature_p);

/*
 * Returns bytes representation of verification key.
 *
 * Note: Returned buffer lifetime is the same as verification key instance.
 *
 * # Arguments
 * * `ver_key` - Verification key instance pointer
 * * `bytes_p` - Pointer that will contains bytes buffer
 * * `bytes_len_p` - Pointer that will contains bytes buffer len
 */
ErrorCode indy_crypto_bls_ver_key_as_bytes(const void *ver_key,
                                           const uint8_t **bytes_p,
                                           size_t *bytes_len_p);

/*
 * Deallocates verification key instance.
 *
 * # Arguments
 * * `ver_key` - Verification key instance pointer
 */
ErrorCode indy_crypto_bls_ver_key_free(const void *ver_key);

/*
 * Creates and returns verification key from bytes representation.
 *
 * Note: Verification key instance deallocation must be performed by calling indy_crypto_bls_very_key_free
 *
 * # Arguments
 * * `bytes` - Bytes buffer pointer
 * * `bytes_len` - Bytes buffer len
 * * `ver_key_p` - Reference that will contain verification key instance pointer
 */
ErrorCode indy_crypto_bls_ver_key_from_bytes(const uint8_t *bytes,
                                             size_t bytes_len,
                                             const void **ver_key_p);

/*
 * Creates and returns BLS ver key that corresponds to sign key.
 *
 * Note: Verification key instance deallocation must be performed by calling indy_crypto_bls_ver_key_free.
 *
 * # Arguments
 * * `gen` - Generator point instance
 * * `sign_key` - Sign key instance
 * * `ver_key_p` - Reference that will contain verification key instance pointer
 */
ErrorCode indy_crypto_bls_ver_key_new(const void *gen,
                                      const void *sign_key,
                                      const void **ver_key_p);

/*
 * Verifies the message multi signature and returns true - if signature valid or false otherwise.
 *
 * # Arguments
 *
 * * `multi_sig` - Multi signature instance pointer
 * * `message` - Message to verify buffer pointer
 * * `message_len` - Message to verify buffer len
 * * `ver_keys` - Verification key instance pointers array
 * * `ver_keys_len` - Verification keys instance pointers array len
 * * `gen` - Generator point instance
 * * `valid_p` - Reference that will be filled with true - if signature valid or false otherwise.
 */
ErrorCode indy_crypto_bls_verify_multi_sig(const void *multi_sig,
                                           const uint8_t *message,
                                           size_t message_len,
                                           const void *const *ver_keys,
                                           size_t ver_keys_len,
                                           const void *gen,
                                           bool *valid_p);

/*
 * Verifies the message signature and returns true - if signature valid or false otherwise.
 *
 * # Arguments
 *
 * * `signature` - Signature instance pointer
 * * `message` - Message to verify buffer pointer
 * * `message_len` - Message to verify buffer len
 * * `ver_key` - Verification key instance pinter
 * * `gen` - Generator instance pointer
 * * `valid_p` - Reference that will be filled with true - if signature valid or false otherwise.
 */
ErrorCode indy_crypto_bsl_verify(const void *signature,
                                 const uint8_t *message,
                                 size_t message_len,
                                 const void *ver_key,
                                 const void *gen,
                                 bool *valid_p);

/*
 * Deallocates blinded master secret correctness proof instance.
 *
 * # Arguments
 * * `blinded_master_secret_correctness_proof` - Reference that contains blinded master secret correctness proof instance pointer.
 */
ErrorCode indy_crypto_cl_blinded_master_secret_correctness_proof_free(const void *blinded_master_secret_correctness_proof);

/*
 * Creates and returns blinded master secret correctness proof json.
 *
 * Note: Blinded master secret correctness proof instance deallocation must be performed
 * by calling indy_crypto_cl_blinded_master_secret_correctness_proof_free.
 *
 * # Arguments
 * * `blinded_master_secret_correctness_proof_json` - Reference that contains blinded master secret correctness proof json.
 * * `blinded_master_secret_correctness_proof_p` - Reference that will contain blinded master secret correctness proof instance pointer.
 */
ErrorCode indy_crypto_cl_blinded_master_secret_correctness_proof_from_json(const char *blinded_master_secret_correctness_proof_json,
                                                                           const void **blinded_master_secret_correctness_proof_p);

/*
 * Returns json representation of blinded master secret correctness proof.
 *
 * # Arguments
 * * `blinded_master_secret_correctness_proof` - Reference that contains blinded master_secret correctness proof pointer.
 * * `blinded_master_secret_correctness_proof_json_p` - Reference that will contain blinded master secret correctness proof json.
 */
ErrorCode indy_crypto_cl_blinded_master_secret_correctness_proof_to_json(const void *blinded_master_secret_correctness_proof,
                                                                         const char **blinded_master_secret_correctness_proof_json_p);

/*
 * Deallocates  blinded master secret instance.
 *
 * # Arguments
 * * `blinded_master_secret` - Reference that contains blinded master secret instance pointer.
 */
ErrorCode indy_crypto_cl_blinded_master_secret_free(const void *blinded_master_secret);

/*
 * Creates and returns blinded master secret from json.
 *
 * Note: Blinded master secret instance deallocation must be performed
 * by calling indy_crypto_cl_blinded_master_secret_free
 *
 * # Arguments
 * * `blinded_master_secret_json` - Reference that contains blinded master secret json.
 * * `blinded_master_secret_p` - Reference that will contain blinded master secret instance pointer.
 */
ErrorCode indy_crypto_cl_blinded_master_secret_from_json(const char *blinded_master_secret_json,
                                                         const void **blinded_master_secret_p);

/*
 * Returns json representation of blinded master secret.
 *
 * # Arguments
 * * `blinded_master_secret` - Reference that contains Blinded master secret pointer.
 * * `blinded_master_secret_json_p` - Reference that will contain blinded master secret json.
 */
ErrorCode indy_crypto_cl_blinded_master_secret_to_json(const void *blinded_master_secret,
                                                       const char **blinded_master_secret_json_p);

/*
 * Deallocates credential key correctness proof instance.
 *
 * # Arguments
 * * `credential_key_correctness_proof` - Reference that contains credential key correctness proof instance pointer.
 */
ErrorCode indy_crypto_cl_credential_key_correctness_proof_free(const void *credential_key_correctness_proof);

/*
 * Creates and returns credential key correctness proof from json.
 *
 * Note: Credential key correctness proof instance deallocation must be performed
 * by calling indy_crypto_cl_credential_key_correctness_proof_free
 *
 * # Arguments
 * * `credential_key_correctness_proof_json` - Reference that contains credential key correctness proof json.
 * * `credential_key_correctness_proof_p` - Reference that will contain credential key correctness proof instance pointer.
 */
ErrorCode indy_crypto_cl_credential_key_correctness_proof_from_json(const char *credential_key_correctness_proof_json,
                                                                    const void **credential_key_correctness_proof_p);

/*
 * Returns json representation of credential key correctness proof.
 *
 * # Arguments
 * * `credential_key_correctness_proof` - Reference that contains credential key correctness proof instance pointer.
 * * `credential_key_correctness_proof_p` - Reference that will contain credential key correctness proof json.
 */
ErrorCode indy_crypto_cl_credential_key_correctness_proof_to_json(const void *credential_key_correctness_proof,
                                                                  const char **credential_key_correctness_proof_json_p);

/*
 * Deallocates credential private key instance.
 *
 * # Arguments
 * * `credential_priv_key` - Reference that contains credential private key instance pointer.
 */
ErrorCode indy_crypto_cl_credential_private_key_free(const void *credential_priv_key);

/*
 * Returns json representation of credential private key.
 *
 * # Arguments
 * * `credential_priv_key` - Reference that contains credential private key instance pointer.
 * * `credential_pub_key_p` - Reference that will contain credential private key json.
 */
ErrorCode indy_crypto_cl_credential_private_key_to_json(const void *credential_priv_key,
                                                        const char **credential_priv_key_json_p);

/*
 * Deallocates credential public key instance.
 *
 * # Arguments
 * * `credential_pub_key` - Reference that contains credential public key instance pointer.
 */
ErrorCode indy_crypto_cl_credential_public_key_free(const void *credential_pub_key);

/*
 * Creates and returns credential public key from json.
 *
 * Note: Credential public key instance deallocation must be performed
 * by calling indy_crypto_cl_credential_public_key_free
 *
 * # Arguments
 * * `credential_pub_key_json` - Reference that contains credential public key json.
 * * `credential_pub_key_p` - Reference that will contain credential public key instance pointer.
 */
ErrorCode indy_crypto_cl_credential_public_key_from_json(const char *credential_pub_key_json,
                                                         const void **credential_pub_key_p);

/*
 * Returns json representation of credential public key.
 *
 * # Arguments
 * * `credential_pub_key` - Reference that contains credential public key instance pointer.
 * * `credential_pub_key_p` - Reference that will contain credential public key json.
 */
ErrorCode indy_crypto_cl_credential_public_key_to_json(const void *credential_pub_key,
                                                       const char **credential_pub_key_json_p);

/*
 * Adds new attribute to credential schema.
 *
 * # Arguments
 * * `credential_schema_builder` - Reference that contains credential schema builder instance pointer.
 * * `attr` - Attribute to add as null terminated string.
 */
ErrorCode indy_crypto_cl_credential_schema_builder_add_attr(const void *credential_schema_builder,
                                                            const char *attr);

/*
 * Deallocates credential schema builder and returns credential schema entity instead.
 *
 * Note: Claims schema instance deallocation must be performed by
 * calling indy_crypto_cl_credential_schema_free.
 *
 * # Arguments
 * * `credential_schema_builder` - Reference that contains credential schema builder instance pointer
 * * `credential_schema_p` - Reference that will contain credentials schema instance pointer.
 */
ErrorCode indy_crypto_cl_credential_schema_builder_finalize(const void *credential_schema_builder,
                                                            const void **credential_schema_p);

/*
 * Creates and returns credential schema entity builder.
 *
 * The purpose of credential schema builder is building of credential schema entity that
 * represents credential schema attributes set.
 *
 * Note: Claim schema builder instance deallocation must be performed by
 * calling indy_crypto_cl_credential_schema_builder_finalize.
 *
 * # Arguments
 * * `credential_schema_builder_p` - Reference that will contain credentials attributes builder instance pointer.
 */
ErrorCode indy_crypto_cl_credential_schema_builder_new(const void **credential_schema_builder_p);

/*
 * Deallocates credential schema instance.
 *
 * # Arguments
 * * `credential_schema` - Reference that contains credential schema instance pointer.
 */
ErrorCode indy_crypto_cl_credential_schema_free(const void *credential_schema);

/*
 * Deallocates credential signature signature instance.
 *
 * # Arguments
 * * `credential_signature` - Reference that contains credential signature instance pointer.
 */
ErrorCode indy_crypto_cl_credential_signature_free(const void *credential_signature);

/*
 * Creates and returns credential signature from json.
 *
 * Note: Credential signature instance deallocation must be performed
 * by calling indy_crypto_cl_credential_signature_free
 *
 * # Arguments
 * * `credential_signature_json` - Reference that contains credential signature json.
 * * `credential_signature_p` - Reference that will contain credential signature instance pointer.
 */
ErrorCode indy_crypto_cl_credential_signature_from_json(const char *credential_signature_json,
                                                        const void **credential_signature_p);

/*
 * Returns json representation of credential signature.
 *
 * # Arguments
 * * `credential_signature` - Reference that contains credential signature pointer.
 * * `credential_signature_json_p` - Reference that will contain credential signature json.
 */
ErrorCode indy_crypto_cl_credential_signature_to_json(const void *credential_signature,
                                                      const char **credential_signature_json_p);

/*
 * Adds new attribute dec_value to credential values map.
 *
 * # Arguments
 * * `credential_values_builder` - Reference that contains credential values builder instance pointer.
 * * `attr` - Claim attr to add as null terminated string.
 * * `dec_value` - Claim attr dec_value. Decimal BigNum representation as null terminated string.
 */
ErrorCode indy_crypto_cl_credential_values_builder_add_value(const void *credential_values_builder,
                                                             const char *attr,
                                                             const char *dec_value);

/*
 * Deallocates credential values builder and returns credential values entity instead.
 *
 * Note: Claims values instance deallocation must be performed by
 * calling indy_crypto_cl_credential_values_free.
 *
 * # Arguments
 * * `credential_values_builder` - Reference that contains credential attribute builder instance pointer.
 * * `credential_values_p` - Reference that will contain credentials values instance pointer.
 */
ErrorCode indy_crypto_cl_credential_values_builder_finalize(const void *credential_values_builder,
                                                            const void **credential_values_p);

/*
 * Creates and returns credentials values entity builder.
 *
 * The purpose of credential values builder is building of credential values entity that
 * represents credential attributes values map.
 *
 * Note: Claims values builder instance deallocation must be performed by
 * calling indy_crypto_cl_credential_values_builder_finalize.
 *
 * # Arguments
 * * `credential_values_builder_p` - Reference that will contain credentials values builder instance pointer.
 */
ErrorCode indy_crypto_cl_credential_values_builder_new(const void **credential_values_builder_p);

/*
 * Deallocates credential values instance.
 *
 * # Arguments
 * * `credential_values` - Claim values instance pointer
 */
ErrorCode indy_crypto_cl_credential_values_free(const void *credential_values);

/*
 * Creates and returns credential definition (public and private keys, correctness proof) entities.
 *
 * Note that credential public key instances deallocation must be performed by
 * calling indy_crypto_cl_credential_public_key_free.
 *
 * Note that credential private key instances deallocation must be performed by
 * calling indy_crypto_cl_credential_private_key_free.
 *
 * Note that credential key correctness proof instances deallocation must be performed by
 * calling indy_crypto_cl_credential_key_correctness_proof_free.
 *
 * # Arguments
 * * `credential_schema` - Reference that contains credential schema instance pointer.
 * * `support_revocation` - If true non revocation part of credential keys will be generated.
 * * `credential_pub_key_p` - Reference that will contain credential public key instance pointer.
 * * `credential_priv_key_p` - Reference that will contain credential private key instance pointer.
 * * `credential_key_correctness_proof_p` - Reference that will contain credential keys correctness proof instance pointer.
 */
ErrorCode indy_crypto_cl_issuer_new_credential_def(const void *credential_schema,
                                                   bool support_revocation,
                                                   const void **credential_pub_key_p,
                                                   const void **credential_priv_key_p,
                                                   const void **credential_key_correctness_proof_p);

/*
 * Creates and returns revocation registries definition (public and private keys, accumulator, tails generator) entities.
 *
 * Note that keys registries deallocation must be performed by
 * calling indy_crypto_cl_revocation_key_public_free and
 * indy_crypto_cl_revocation_key_private_free.
 *
 * Note that accumulator deallocation must be performed by
 * calling indy_crypto_cl_revocation_registry_free.
 *
 * Note that tails generator deallocation must be performed by
 * calling indy_crypto_cl_revocation_tails_generator_free.
 *
 * # Arguments
 * * `credential_pub_key` - Reference that contains credential pub key instance pointer.
 * * `max_cred_num` - Max credential number in generated registry.
 * * `issuance_by_default` - Type of issuance.
 * If true all indices are assumed to be issued and initial accumulator is calculated over all indices
 * If false nothing is issued initially accumulator is 1
 * * `rev_key_pub_p` - Reference that will contain revocation key public instance pointer.
 * * `rev_key_priv_p` - Reference that will contain revocation key private instance pointer.
 * * `rev_reg_p` - Reference that will contain revocation registry instance pointer.
 * * `rev_tails_generator_p` - Reference that will contain revocation tails generator instance pointer.
 */
ErrorCode indy_crypto_cl_issuer_new_revocation_registry_def(const void *credential_pub_key,
                                                            uint32_t max_cred_num,
                                                            bool issuance_by_default,
                                                            const void **rev_key_pub_p,
                                                            const void **rev_key_priv_p,
                                                            const void **rev_reg_p,
                                                            const void **rev_tails_generator_p);

/*
 * Creates and returns credential private key from json.
 *
 * Note: Credential private key instance deallocation must be performed
 * by calling indy_crypto_cl_credential_private_key_free
 *
 * # Arguments
 * * `credential_priv_key_json` - Reference that contains credential private key json.
 * * `credential_priv_key_p` - Reference that will contain credential private key instance pointer.
 */
ErrorCode indy_crypto_cl_issuer_private_key_from_json(const char *credential_priv_key_json,
                                                      const void **credential_priv_key_p);

/*
 * Recovery a credential by a rev_idx in a given revocation registry
 *
 * # Arguments
 * * `rev_reg` - Reference that contain revocation registry instance pointer.
 *  * max_cred_num` - Max credential number in revocation registry.
 *  * rev_idx` - Index of the user in the revocation registry.
 */
ErrorCode indy_crypto_cl_issuer_recovery_credential(const void *rev_reg,
                                                    uint32_t max_cred_num,
                                                    uint32_t rev_idx,
                                                    const void *ctx_tails,
                                                    FFITailTake take_tail,
                                                    FFITailPut put_tail,
                                                    const void **rev_reg_delta_p);

/*
 * Revokes a credential by a rev_idx in a given revocation registry.
 *
 * # Arguments
 * * `rev_reg` - Reference that contain revocation registry instance pointer.
 *  * max_cred_num` - Max credential number in revocation registry.
 *  * rev_idx` - Index of the user in the revocation registry.
 */
ErrorCode indy_crypto_cl_issuer_revoke_credential(const void *rev_reg,
                                                  uint32_t max_cred_num,
                                                  uint32_t rev_idx,
                                                  const void *ctx_tails,
                                                  FFITailTake take_tail,
                                                  FFITailPut put_tail,
                                                  const void **rev_reg_delta_p);

/*
 * Signs credential values with primary keys only.
 *
 * Note that credential signature instances deallocation must be performed by
 * calling indy_crypto_cl_credential_signature_free.
 *
 * Note that credential signature correctness proof instances deallocation must be performed by
 * calling indy_crypto_cl_signature_correctness_proof_free.
 *
 * # Arguments
 * * `prover_id` - Prover identifier.
 * * `blinded_master_secret` - Blinded master secret instance pointer generated by Prover.
 * * `blinded_master_secret_correctness_proof` - Blinded master secret correctness proof instance pointer.
 * * `master_secret_blinding_nonce` - Nonce instance pointer used for verification of blinded_master_secret_correctness_proof.
 * * `credential_issuance_nonce` - Nonce instance pointer used for creation of signature_correctness_proof.
 * * `credential_values` - Claim values to be signed instance pointer.
 * * `credential_pub_key` - Credential public key instance pointer.
 * * `credential_priv_key` - Credential private key instance pointer.
 * * `credential_signature_p` - Reference that will contain credential signature instance pointer.
 * * `credential_signature_correctness_proof_p` - Reference that will contain credential signature correctness proof instance pointer.
 */
ErrorCode indy_crypto_cl_issuer_sign_credential(const char *prover_id,
                                                const void *blinded_master_secret,
                                                const void *blinded_master_secret_correctness_proof,
                                                const void *master_secret_blinding_nonce,
                                                const void *credential_issuance_nonce,
                                                const void *credential_values,
                                                const void *credential_pub_key,
                                                const void *credential_priv_key,
                                                const void **credential_signature_p,
                                                const void **credential_signature_correctness_proof_p);

/*
 * Signs credential values with both primary and revocation keys.
 *
 *
 * Note that credential signature instances deallocation must be performed by
 * calling indy_crypto_cl_credential_signature_free.
 *
 * Note that credential signature correctness proof instances deallocation must be performed by
 * calling indy_crypto_cl_signature_correctness_proof_free.
 *
 *
 * Note that credential signature correctness proof instances deallocation must be performed by
 * calling indy_crypto_cl_revocation_registry_delta_free.
 *
 * # Arguments
 * * `prover_id` - Prover identifier.
 * * `blinded_master_secret` - Blinded master secret instance pointer generated by Prover.
 * * `blinded_master_secret_correctness_proof` - Blinded master secret correctness proof instance pointer.
 * * `master_secret_blinding_nonce` - Nonce instance pointer used for verification of blinded_master_secret_correctness_proof.
 * * `credential_issuance_nonce` - Nonce instance pointer used for creation of signature_correctness_proof.
 * * `credential_values` - Claim values to be signed instance pointer.
 * * `credential_pub_key` - Credential public key instance pointer.
 * * `credential_priv_key` - Credential private key instance pointer.
 * * `rev_idx` - User index in revocation accumulator. Required for non-revocation credential_signature part generation.
 * * `max_cred_num` - Max credential number in generated registry.
 * * `rev_reg` - Revocation registry instance pointer.
 * * `rev_key_priv` - Revocation registry private key instance pointer.
 * * `credential_signature_p` - Reference that will contain credential signature instance pointer.
 * * `credential_signature_correctness_proof_p` - Reference that will contain credential signature correctness proof instance pointer.
 * * `revocation_registry_delta_p` - Reference that will contain revocation registry delta instance pointer.
 */
ErrorCode indy_crypto_cl_issuer_sign_credential_with_revoc(const char *prover_id,
                                                           const void *blinded_master_secret,
                                                           const void *blinded_master_secret_correctness_proof,
                                                           const void *master_secret_blinding_nonce,
                                                           const void *credential_issuance_nonce,
                                                           const void *credential_values,
                                                           const void *credential_pub_key,
                                                           const void *credential_priv_key,
                                                           uint32_t rev_idx,
                                                           uint32_t max_cred_num,
                                                           bool issuance_by_default,
                                                           const void *rev_reg,
                                                           const void *rev_key_priv,
                                                           const void *ctx_tails,
                                                           FFITailTake take_tail,
                                                           FFITailPut put_tail,
                                                           const void **credential_signature_p,
                                                           const void **credential_signature_correctness_proof_p,
                                                           const void **revocation_registry_delta_p);

/*
 * Deallocates master secret blinding data instance.
 *
 * # Arguments
 * * `master_secret_blinding_data` - Reference that contains master secret  blinding data instance pointer.
 */
ErrorCode indy_crypto_cl_master_secret_blinding_data_free(const void *master_secret_blinding_data);

/*
 * Creates and returns master secret blinding data json.
 *
 * Note: Master secret blinding data instance deallocation must be performed
 * by calling indy_crypto_cl_master_secret_blinding_data_free.
 *
 * # Arguments
 * * `master_secret_blinding_data_json` - Reference that contains master secret blinding data json.
 * * `blinded_master_secret_p` - Reference that will contain master secret blinding data instance pointer.
 */
ErrorCode indy_crypto_cl_master_secret_blinding_data_from_json(const char *master_secret_blinding_data_json,
                                                               const void **master_secret_blinding_data_p);

/*
 * Returns json representation of master secret blinding data.
 *
 * # Arguments
 * * `master_secret_blinding_data` - Reference that contains master secret blinding data pointer.
 * * `master_secret_blinding_data_json_p` - Reference that will contain master secret blinding data json.
 */
ErrorCode indy_crypto_cl_master_secret_blinding_data_to_json(const void *master_secret_blinding_data,
                                                             const char **master_secret_blinding_data_json_p);

/*
 * Deallocates master secret instance.
 *
 * # Arguments
 * * `master_secret` - Reference that contains master secret instance pointer.
 */
ErrorCode indy_crypto_cl_master_secret_free(const void *master_secret);

/*
 * Creates and returns master secret from json.
 *
 * Note: Master secret instance deallocation must be performed
 * by calling indy_crypto_cl_master_secret_free.
 *
 * # Arguments
 * * `master_secret_json` - Reference that contains master secret json.
 * * `master_secret_p` - Reference that will contain master secret instance pointer.
 */
ErrorCode indy_crypto_cl_master_secret_from_json(const char *master_secret_json,
                                                 const void **master_secret_p);

/*
 * Returns json representation of master secret.
 *
 * # Arguments
 * * `master_secret` - Reference that contains master secret instance pointer.
 * * `master_secret_json_p` - Reference that will contain master secret json.
 */
ErrorCode indy_crypto_cl_master_secret_to_json(const void *master_secret,
                                               const char **master_secret_json_p);

/*
 * Creates random nonce.
 *
 * Note that nonce deallocation must be performed by calling indy_crypto_cl_nonce_free.
 *
 * # Arguments
 * * `nonce_p` - Reference that will contain nonce instance pointer.
 */
ErrorCode indy_crypto_cl_new_nonce(const void **nonce_p);

/*
 * Deallocates nonce instance.
 *
 * # Arguments
 * * `nonce` - Reference that contains nonce instance pointer.
 */
ErrorCode indy_crypto_cl_nonce_free(const void *nonce);

/*
 * Creates and returns nonce json.
 *
 * Note: Nonce instance deallocation must be performed by calling indy_crypto_cl_nonce_free.
 *
 * # Arguments
 * * `nonce_json` - Reference that contains nonce json.
 * * `nonce_p` - Reference that will contain nonce instance pointer.
 */
ErrorCode indy_crypto_cl_nonce_from_json(const char *nonce_json, const void **nonce_p);

/*
 * Returns json representation of nonce.
 *
 * # Arguments
 * * `nonce` - Reference that contains nonce instance pointer.
 * * `nonce_json_p` - Reference that will contain nonce json.
 */
ErrorCode indy_crypto_cl_nonce_to_json(const void *nonce, const char **nonce_json_p);

ErrorCode indy_crypto_cl_proof_builder_add_sub_proof_request(const void *proof_builder,
                                                             const char *key_id,
                                                             const void *sub_proof_request,
                                                             const void *credential_schema,
                                                             const void *credential_signature,
                                                             const void *credential_values,
                                                             const void *credential_pub_key,
                                                             const void *rev_reg,
                                                             const void *witness);

/*
 * Finalize proof.
 *
 * Note that proof deallocation must be performed by
 * calling indy_crypto_cl_proof_free.
 *
 * # Arguments
 * * `proof_builder` - Reference that contain proof builder instance pointer.
 * * `nonce` - Reference that contain nonce instance pointer.
 * * `master_secret` - Reference that contain master secret instance pointer.
 * * `proof_p` - Reference that will contain proof instance pointer.
 */
ErrorCode indy_crypto_cl_proof_builder_finalize(const void *proof_builder,
                                                const void *nonce,
                                                const void *master_secret,
                                                const void **proof_p);

/*
 * Deallocates proof instance.
 *
 * # Arguments
 * * `proof` - Reference that contains proof instance pointer.
 */
ErrorCode indy_crypto_cl_proof_free(const void *proof);

/*
 * Creates and returns proof json.
 *
 * Note: Proof instance deallocation must be performed by calling indy_crypto_cl_proof_free.
 *
 * # Arguments
 * * `proof_json` - Reference that contains proof json.
 * * `proof_p` - Reference that will contain proof instance pointer.
 */
ErrorCode indy_crypto_cl_proof_from_json(const char *proof_json, const void **proof_p);

/*
 * Returns json representation of proof.
 *
 * # Arguments
 * * `proof` - Reference that contains proof instance pointer.
 * * `proof_json_p` - Reference that will contain proof json.
 */
ErrorCode indy_crypto_cl_proof_to_json(const void *proof, const char **proof_json_p);

ErrorCode indy_crypto_cl_proof_verifier_add_sub_proof_request(const void *proof_verifier,
                                                              const char *key_id,
                                                              const void *sub_proof_request,
                                                              const void *credential_schema,
                                                              const void *credential_pub_key,
                                                              const void *rev_key_pub,
                                                              const void *rev_reg);

/*
 * Verifies proof and deallocates proof verifier.
 *
 * # Arguments
 * * `proof_verifier` - Reference that contain proof verifier instance pointer.
 * * `proof` - Reference that contain proof instance pointer.
 * * `nonce` - Reference that contain nonce instance pointer.
 * * `valid_p` - Reference that will be filled with true - if proof valid or false otherwise.
 */
ErrorCode indy_crypto_cl_proof_verifier_verify(const void *proof_verifier,
                                               const void *proof,
                                               const void *nonce,
                                               bool *valid_p);

/*
 * Creates blinded master secret for given issuer key and master secret.
 *
 * Note that blinded master secret deallocation must be performed by
 * calling indy_crypto_cl_blinded_master_secret_free.
 *
 * Note that master secret blinding data deallocation must be performed by
 * calling indy_crypto_cl_master_secret_blinding_data_free.
 *
 * Note that blinded master secret proof correctness deallocation must be performed by
 * calling indy_crypto_cl_blinded_master_secret_correctness_proof_free.
 *
 * # Arguments
 * * `credential_pub_key` - Reference that contains credential public key instance pointer.
 * * `credential_key_correctness_proof` - Reference that contains credential key correctness proof instance pointer.
 * * `master_secret` - Reference that contains master secret instance pointer.
 * * `master_secret_blinding_nonce` - Reference that contains nonce instance pointer.
 * * `blinded_master_secret_p` - Reference that will contain blinded master secret instance pointer.
 * * `master_secret_blinding_data_p` - Reference that will contain master secret blinding data instance pointer.
 * * `blinded_master_secret_correctness_proof_p` - Reference that will contain blinded master secret correctness proof instance pointer.
 */
ErrorCode indy_crypto_cl_prover_blind_master_secret(const void *credential_pub_key,
                                                    const void *credential_key_correctness_proof,
                                                    const void *master_secret,
                                                    const void *master_secret_blinding_nonce,
                                                    const void **blinded_master_secret_p,
                                                    const void **master_secret_blinding_data_p,
                                                    const void **blinded_master_secret_correctness_proof_p);

/*
 * Creates a master secret.
 *
 * Note that master secret deallocation must be performed by
 * calling indy_crypto_cl_master_secret_free.
 *
 * # Arguments
 * * `master_secret_p` - Reference that will contain master secret instance pointer.
 */
ErrorCode indy_crypto_cl_prover_new_master_secret(const void **master_secret_p);

/*
 * Creates and returns proof builder.
 *
 * The purpose of proof builder is building of proof entity according to the given request .
 *
 * Note that proof builder deallocation must be performed by
 * calling indy_crypto_cl_proof_builder_finalize.
 *
 * # Arguments
 * * `proof_builder_p` - Reference that will contain proof builder instance pointer.
 */
ErrorCode indy_crypto_cl_prover_new_proof_builder(const void **proof_builder_p);

/*
 * Updates the credential signature by a master secret blinding data.
 *
 * # Arguments
 * * `credential_signature` - Credential signature instance pointer generated by Issuer.
 * * `credential_values` - Credential values instance pointer.
 * * `signature_correctness_proof` - Credential signature correctness proof instance pointer.
 * * `master_secret_blinding_data` - Master secret blinding data instance pointer.
 * * `master_secret` - Master secret instance pointer.
 * * `credential_pub_key` - Credential public key instance pointer.
 * * `nonce` -  Nonce instance pointer was used by Issuer for the creation of signature_correctness_proof.
 * * `rev_key_pub` - (Optional) Revocation registry public key  instance pointer.
 * * `rev_reg` - (Optional) Revocation registry  instance pointer.
 * * `witness` - (Optional) Witness instance pointer.
 */
ErrorCode indy_crypto_cl_prover_process_credential_signature(const void *credential_signature,
                                                             const void *credential_values,
                                                             const void *signature_correctness_proof,
                                                             const void *master_secret_blinding_data,
                                                             const void *master_secret,
                                                             const void *credential_pub_key,
                                                             const void *credential_issuance_nonce,
                                                             const void *rev_key_pub,
                                                             const void *rev_reg,
                                                             const void *witness);

/*
 * Deallocates revocation key private instance.
 *
 * # Arguments
 * * `rev_key_priv` - Reference that contains revocation key private instance pointer.
 */
ErrorCode indy_crypto_cl_revocation_key_private_free(const void *rev_key_priv);

/*
 * Creates and returns revocation key private from json.
 *
 * Note: Revocation registry private instance deallocation must be performed
 * by calling indy_crypto_cl_revocation_key_private_free
 *
 * # Arguments
 * * `rev_key_priv_json` - Reference that contains revocation key private json.
 * * `rev_key_priv_p` - Reference that will contain revocation key private instance pointer
 */
ErrorCode indy_crypto_cl_revocation_key_private_from_json(const char *rev_key_priv_json,
                                                          const void **rev_key_priv_p);

/*
 * Returns json representation of revocation key private.
 *
 * # Arguments
 * * `rev_key_priv` - Reference that contains issuer revocation key private pointer.
 * * `rev_key_priv_json_p` - Reference that will contain revocation key private json
 */
ErrorCode indy_crypto_cl_revocation_key_private_to_json(const void *rev_key_priv,
                                                        const char **rev_key_priv_json_p);

/*
 * Deallocates revocation key public instance.
 *
 * # Arguments
 * * `rev_key_pub` - Reference that contains revocation key public instance pointer.
 */
ErrorCode indy_crypto_cl_revocation_key_public_free(const void *rev_key_pub);

/*
 * Creates and returns revocation key public from json.
 *
 * Note: Revocation registry public instance deallocation must be performed
 * by calling indy_crypto_cl_revocation_key_public_free
 *
 * # Arguments
 * * `rev_key_pub_json` - Reference that contains revocation key public json.
 * * `rev_key_pub_p` - Reference that will contain revocation key public instance pointer.
 */
ErrorCode indy_crypto_cl_revocation_key_public_from_json(const char *rev_key_pub_json,
                                                         const void **rev_key_pub_p);

/*
 * Returns json representation of revocation key public.
 *
 * # Arguments
 * * `rev_key_pub` - Reference that contains revocation key public pointer.
 * * `rev_key_pub_json_p` - Reference that will contain revocation key public json.
 */
ErrorCode indy_crypto_cl_revocation_key_public_to_json(const void *rev_key_pub,
                                                       const char **rev_key_pub_json_p);

/*
 * Deallocates revocation registry delta instance.
 *
 * # Arguments
 * * `revocation_registry_delta` - Reference that contains revocation registry delta instance pointer.
 */
ErrorCode indy_crypto_cl_revocation_registry_delta_free(const void *revocation_registry_delta);

/*
 * Creates and returns revocation registry delta from json.
 *
 * Note: Revocation registry delta instance deallocation must be performed
 * by calling indy_crypto_cl_revocation_registry_delta_free
 *
 * # Arguments
 * * `revocation_registry_delta_json` - Reference that contains revocation registry delta json.
 * * `revocation_registry_delta_p` - Reference that will contain revocation registry delta instance pointer.
 */
ErrorCode indy_crypto_cl_revocation_registry_delta_from_json(const char *revocation_registry_delta_json,
                                                             const void **revocation_registry_delta_p);

/*
 * Returns json representation of revocation registry delta.
 *
 * # Arguments
 * * `revocation_registry_delta` - Reference that contains revocation registry delta instance pointer.
 * * `revocation_registry_delta_json_p` - Reference that will contain revocation registry delta json.
 */
ErrorCode indy_crypto_cl_revocation_registry_delta_to_json(const void *revocation_registry_delta,
                                                           const char **revocation_registry_delta_json_p);

/*
 * Deallocates revocation registry instance.
 *
 * # Arguments
 * * `rev_reg` - Reference that contains revocation registry instance pointer.
 */
ErrorCode indy_crypto_cl_revocation_registry_free(const void *rev_reg);

/*
 * Creates and returns revocation registry from json.
 *
 * Note: Revocation registry instance deallocation must be performed
 * by calling indy_crypto_cl_revocation_registry_free
 *
 * # Arguments
 * * `rev_reg_json` - Reference that contains revocation registry json.
 * * `rev_reg_p` - Reference that will contain revocation registry instance pointer
 */
ErrorCode indy_crypto_cl_revocation_registry_from_json(const char *rev_reg_json,
                                                       const void **rev_reg_p);

/*
 * Returns json representation of revocation registry.
 *
 * # Arguments
 * * `rev_reg` - Reference that contains revocation registry pointer.
 * * `rev_reg_p` - Reference that will contain revocation registry json
 */
ErrorCode indy_crypto_cl_revocation_registry_to_json(const void *rev_reg,
                                                     const char **rev_reg_json_p);

/*
 * Deallocates revocation tails generator instance.
 *
 * # Arguments
 * * `rev_tails_generator` - Reference that contains revocation tails generator instance pointer.
 */
ErrorCode indy_crypto_cl_revocation_tails_generator_free(const void *rev_tails_generator);

/*
 * Creates and returns revocation tails generator from json.
 *
 * Note: Revocation tails generator instance deallocation must be performed
 * by calling indy_crypto_cl_revocation_tails_generator_free
 *
 * # Arguments
 * * `rev_tails_generator_json` - Reference that contains revocation tails generator json.
 * * `rev_tails_generator_p` - Reference that will contain revocation tails generator instance pointer
 */
ErrorCode indy_crypto_cl_revocation_tails_generator_from_json(const char *rev_tails_generator_json,
                                                              const void **rev_tails_generator_p);

/*
 * Returns json representation of revocation tails generator.
 *
 * # Arguments
 * * `rev_tails_generator` - Reference that contains revocation tails generator pointer.
 * * `rev_tails_generator_p` - Reference that will contain revocation tails generator json
 */
ErrorCode indy_crypto_cl_revocation_tails_generator_to_json(const void *rev_tails_generator,
                                                            const char **rev_tails_generator_json_p);

/*
 * Deallocates signature correctness proof instance.
 *
 * # Arguments
 * * `signature_correctness_proof` - Reference that contains signature correctness proof instance pointer.
 */
ErrorCode indy_crypto_cl_signature_correctness_proof_free(const void *signature_correctness_proof);

/*
 * Creates and returns signature correctness proof from json.
 *
 * Note: Signature correctness proof instance deallocation must be performed
 * by calling indy_crypto_cl_signature_correctness_proof_free
 *
 * # Arguments
 * * `signature_correctness_proof_json` - Reference that contains signature correctness proof json.
 * * `signature_correctness_proof_p` - Reference that will contain signature correctness proof instance pointer.
 */
ErrorCode indy_crypto_cl_signature_correctness_proof_from_json(const char *signature_correctness_proof_json,
                                                               const void **signature_correctness_proof_p);

/*
 * Returns json representation of signature correctness proof.
 *
 * # Arguments
 * * `signature_correctness_proof` - Reference that contains signature correctness proof instance pointer.
 * * `signature_correctness_proof_json_p` - Reference that will contain signature correctness proof json.
 */
ErrorCode indy_crypto_cl_signature_correctness_proof_to_json(const void *signature_correctness_proof,
                                                             const char **signature_correctness_proof_json_p);

/*
 * Adds predicate to sub proof request.
 *
 * # Arguments
 * * `sub_proof_request_builder` - Reference that contains sub proof request builder instance pointer.
 * * `attr_name` - Related attribute
 * * `p_type` - Predicate type (Currently `GE` only).
 * * `value` - Requested value.
 */
ErrorCode indy_crypto_cl_sub_proof_request_builder_add_predicate(const void *sub_proof_request_builder,
                                                                 const char *attr_name,
                                                                 const char *p_type,
                                                                 int32_t value);

/*
 * Adds new revealed attribute to sub proof request.
 *
 * # Arguments
 * * `sub_proof_request_builder` - Reference that contains sub proof request builder instance pointer.
 * * `attr` - Claim attr to add as null terminated string.
 */
ErrorCode indy_crypto_cl_sub_proof_request_builder_add_revealed_attr(const void *sub_proof_request_builder,
                                                                     const char *attr);

/*
 * Deallocates sub proof request builder and returns sub proof request entity instead.
 *
 * Note: Sub proof request instance deallocation must be performed by
 * calling indy_crypto_cl_sub_proof_request_free.
 *
 * # Arguments
 * * `sub_proof_request_builder` - Reference that contains sub proof request builder instance pointer.
 * * `sub_proof_request_p` - Reference that will contain sub proof request instance pointer.
 */
ErrorCode indy_crypto_cl_sub_proof_request_builder_finalize(const void *sub_proof_request_builder,
                                                            const void **sub_proof_request_p);

/*
 * Creates and returns sub proof request entity builder.
 *
 * The purpose of sub proof request builder is building of sub proof request entity that
 * represents requested attributes and predicates.
 *
 * Note: sub proof request builder instance deallocation must be performed by
 * calling indy_crypto_cl_sub_proof_request_builder_finalize.
 *
 * # Arguments
 * * `sub_proof_request_builder_p` - Reference that will contain sub proof request builder instance pointer.
 */
ErrorCode indy_crypto_cl_sub_proof_request_builder_new(const void **sub_proof_request_builder_p);

/*
 * Deallocates sub proof request instance.
 *
 * # Arguments
 * * `sub_proof_request` - Reference that contains sub proof request instance pointer.
 */
ErrorCode indy_crypto_cl_sub_proof_request_free(const void *sub_proof_request);

ErrorCode indy_crypto_cl_tail_free(const void *tail);

ErrorCode indy_crypto_cl_tails_generator_count(const void *rev_tails_generator, uint32_t *count_p);

ErrorCode indy_crypto_cl_tails_generator_next(const void *rev_tails_generator, const void **tail_p);

/*
 * Creates and returns proof verifier.
 *
 * Note that proof verifier deallocation must be performed by
 * calling indy_crypto_cl_proof_verifier_finalize.
 *
 * # Arguments
 * * `proof_verifier_p` - Reference that will contain proof verifier instance pointer.
 */
ErrorCode indy_crypto_cl_verifier_new_proof_verifier(const void **proof_verifier_p);

ErrorCode indy_crypto_cl_witness_free(const void *witness);

ErrorCode indy_crypto_cl_witness_new(uint32_t rev_idx,
                                     uint32_t max_cred_num,
                                     const void *rev_reg_delta,
                                     const void *ctx_tails,
                                     FFITailTake take_tail,
                                     FFITailPut put_tail,
                                     const void **witness_p);

ErrorCode indy_crypto_cl_witness_update(uint32_t rev_idx,
                                        uint32_t max_cred_num,
                                        const void *rev_reg_delta,
                                        void *witness,
                                        const void *ctx_tails,
                                        FFITailTake take_tail,
                                        FFITailPut put_tail);

void indy_crypto_init_logger(void);
